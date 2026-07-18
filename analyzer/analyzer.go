package analyzer

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/orm"
	"github.com/dionisius77/dorm/schema"
)

const DefaultLargeOffsetThreshold = 1000

type QueryKind string

const (
	QueryKindUnknown QueryKind = "unknown"
	QueryKindSelect  QueryKind = "select"
	QueryKindUpdate  QueryKind = "update"
	QueryKindDelete  QueryKind = "delete"
	QueryKindInsert  QueryKind = "insert"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type Finding struct {
	Code           string
	Rule           string
	Severity       Severity
	Title          string
	Details        string
	Recommendation string
	Table          string
	Columns        []string
}

type Report struct {
	Table    string
	Kind     QueryKind
	SQL      string
	Findings []Finding
}

func (r Report) Empty() bool { return len(r.Findings) == 0 }

func (r Report) String() string {
	if len(r.Findings) == 0 {
		return "✓ Query looks healthy"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "⚠ Query analysis found %d issue(s)\n", len(r.Findings))
	for _, finding := range r.Findings {
		fmt.Fprintf(&b, "- %s: %s\n", finding.Title, finding.Details)
		if finding.Recommendation != "" {
			fmt.Fprintf(&b, "  Recommendation: %s\n", finding.Recommendation)
		}
	}
	return b.String()
}

type Input struct {
	SQL    string
	Table  string
	Schema *schema.Schema
}

type Rule interface {
	Name() string
	Apply(context Context) []Finding
}

type Analyzer struct {
	rules                []Rule
	largeOffsetThreshold int
	obs                  orm.ObservabilityConfig
}

type Config struct {
	Rules                []Rule
	LargeOffsetThreshold int
	Observability        orm.ObservabilityConfig
}

func New(cfg Config) *Analyzer {
	rules := cfg.Rules
	if len(rules) == 0 {
		rules = DefaultRules()
	}
	threshold := cfg.LargeOffsetThreshold
	if threshold <= 0 {
		threshold = DefaultLargeOffsetThreshold
	}
	return &Analyzer{
		rules:                append([]Rule(nil), rules...),
		largeOffsetThreshold: threshold,
		obs:                  cfg.Observability.Normalized(),
	}
}

func DefaultRules() []Rule {
	return []Rule{
		RuleMissingWhere{},
		RuleSequentialScan{},
		RuleMissingIndex{},
		RuleMissingCompositeIndex{},
		RuleLargeOffset{},
		RulePotentialFullTableScan{},
		RuleInefficientOrderBy{},
	}
}

func (a *Analyzer) Analyze(ctx context.Context, input Input) (Report, error) {
	if a == nil {
		a = New(Config{})
	}
	var report Report
	err := a.trace(ctx, "query.analyze", func(ctx context.Context, span orm.Span) error {
		parsed, table, err := a.buildContext(input)
		if err != nil {
			return err
		}
		report = Report{
			Table: table.Name,
			Kind:  parsed.Kind,
			SQL:   parsed.SQL,
		}
		c := Context{
			Query:             parsed,
			Table:             table,
			Schema:            input.Schema,
			LargeOffsetThresh: a.largeOffsetThreshold,
		}
		for _, rule := range a.rules {
			findings := rule.Apply(c)
			report.Findings = append(report.Findings, findings...)
		}
		sort.SliceStable(report.Findings, func(i, j int) bool {
			if report.Findings[i].Severity == report.Findings[j].Severity {
				return report.Findings[i].Title < report.Findings[j].Title
			}
			return severityRank(report.Findings[i].Severity) < severityRank(report.Findings[j].Severity)
		})
		if span != nil {
			span.SetAttributes(
				orm.Attribute{Key: "orm.analyzer.table", Value: report.Table},
				orm.Attribute{Key: "orm.analyzer.kind", Value: string(report.Kind)},
				orm.Attribute{Key: "orm.analyzer.findings", Value: len(report.Findings)},
			)
		}
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	return report, nil
}

// Inspect adapts the analyzer to the ORM query advisor interface.
func (a *Analyzer) Inspect(ctx context.Context, input orm.QueryAdvisorInput) (orm.QueryAdvisorReport, error) {
	report, err := a.Analyze(ctx, Input{
		SQL:    input.SQL,
		Table:  input.Table,
		Schema: input.Schema,
	})
	if err != nil {
		return orm.QueryAdvisorReport{}, err
	}
	out := orm.QueryAdvisorReport{
		Table: report.Table,
		SQL:   report.SQL,
		Kind:  string(report.Kind),
	}
	for _, finding := range report.Findings {
		out.Findings = append(out.Findings, orm.QueryAdvisorFinding{
			Code:           finding.Code,
			Rule:           finding.Rule,
			Severity:       string(finding.Severity),
			Title:          finding.Title,
			Details:        finding.Details,
			Recommendation: finding.Recommendation,
			Table:          finding.Table,
			Columns:        append([]string(nil), finding.Columns...),
		})
	}
	return out, nil
}

func (a *Analyzer) buildContext(input Input) (ParsedQuery, *schema.Table, error) {
	if strings.TrimSpace(input.SQL) == "" {
		return ParsedQuery{}, nil, errorf("query analyzer requires SQL input")
	}
	if input.Schema == nil {
		return ParsedQuery{}, nil, errorf("query analyzer requires schema metadata")
	}
	parsed, err := ParseSQL(input.SQL)
	if err != nil {
		return ParsedQuery{}, nil, err
	}
	tableName := strings.TrimSpace(input.Table)
	if tableName == "" {
		tableName = parsed.Table
	}
	if tableName == "" && len(input.Schema.Tables) == 1 {
		tableName = input.Schema.Tables[0].Name
	}
	if tableName == "" {
		return ParsedQuery{}, nil, errorf("query analyzer requires a table name when SQL does not identify one")
	}
	table := findTable(input.Schema, tableName)
	if table == nil {
		return ParsedQuery{}, nil, errorf("query analyzer could not find table %q in schema metadata", tableName)
	}
	parsed.Table = table.Name
	return parsed, table, nil
}

func (a *Analyzer) trace(ctx context.Context, spanName string, fn func(context.Context, orm.Span) error) error {
	if a == nil || !a.obs.Tracing {
		return fn(ctx, nil)
	}
	if a.obs.TracerProvider == nil {
		return fn(ctx, nil)
	}
	ctx, span := a.obs.TracerProvider.Tracer("github.com/dionisius77/dorm").Start(ctx, spanName)
	err := fn(ctx, span)
	if err != nil {
		span.RecordError(err)
		if a.obs.Logger != nil {
			a.obs.Logger.LogError(ctx, err)
		}
	}
	span.End()
	return err
}

type Context struct {
	Query             ParsedQuery
	Table             *schema.Table
	Schema            *schema.Schema
	LargeOffsetThresh int
}

type ParsedQuery struct {
	SQL           string
	Kind          QueryKind
	Table         string
	WhereClause   string
	OrderByClause string
	WhereColumns  []string
	OrderByCols   []string
	Offset        int
	HasOffset     bool
	HasWhere      bool
	HasJoin       bool
}

func ParseSQL(sqlText string) (ParsedQuery, error) {
	normalized := strings.TrimSpace(sqlText)
	normalized = strings.TrimSuffix(normalized, ";")
	if normalized == "" {
		return ParsedQuery{}, errorf("query analyzer requires SQL input")
	}
	lower := strings.ToLower(normalized)
	pq := ParsedQuery{SQL: normalized, Offset: -1}
	switch {
	case strings.HasPrefix(lower, "select "):
		pq.Kind = QueryKindSelect
		pq.Table = extractTable(normalized, `(?is)\bfrom\s+([a-zA-Z0-9_".]+)`)
	case strings.HasPrefix(lower, "update "):
		pq.Kind = QueryKindUpdate
		pq.Table = extractTable(normalized, `(?is)^\s*update\s+([a-zA-Z0-9_".]+)`)
	case strings.HasPrefix(lower, "delete "):
		pq.Kind = QueryKindDelete
		pq.Table = extractTable(normalized, `(?is)^\s*delete\s+from\s+([a-zA-Z0-9_".]+)`)
	case strings.HasPrefix(lower, "insert "):
		pq.Kind = QueryKindInsert
		pq.Table = extractTable(normalized, `(?is)^\s*insert\s+into\s+([a-zA-Z0-9_".]+)`)
	}
	if strings.Contains(lower, " join ") {
		pq.HasJoin = true
	}
	if where, ok := extractClause(normalized, "where"); ok {
		pq.WhereClause = where
		pq.HasWhere = true
		pq.WhereColumns = extractWhereColumns(where)
	}
	if order, ok := extractClause(normalized, "order by"); ok {
		pq.OrderByClause = order
		pq.OrderByCols = extractOrderByColumns(order)
	}
	if off, ok := extractOffset(normalized); ok {
		pq.Offset = off
		pq.HasOffset = true
	}
	return pq, nil
}

type RuleMissingWhere struct{}

func (RuleMissingWhere) Name() string { return "missing_where" }

func (RuleMissingWhere) Apply(ctx Context) []Finding {
	if ctx.Query.Kind != QueryKindSelect && ctx.Query.Kind != QueryKindUpdate && ctx.Query.Kind != QueryKindDelete {
		return nil
	}
	if ctx.Query.HasWhere {
		return nil
	}
	return []Finding{{
		Code:           "missing_where",
		Rule:           "Missing WHERE",
		Severity:       SeverityCritical,
		Title:          "Missing WHERE",
		Details:        "The query does not filter rows.",
		Recommendation: "Add a WHERE clause to limit the rows scanned by PostgreSQL.",
		Table:          ctx.Table.Name,
	}}
}

type RuleSequentialScan struct{}

func (RuleSequentialScan) Name() string { return "sequential_scan" }

func (RuleSequentialScan) Apply(ctx Context) []Finding {
	if ctx.Query.Kind != QueryKindSelect && ctx.Query.Kind != QueryKindUpdate && ctx.Query.Kind != QueryKindDelete {
		return nil
	}
	effective := effectiveFilterColumns(ctx)
	if len(effective) == 0 {
		return []Finding{{
			Code:           "sequential_scan",
			Rule:           "Sequential Scan",
			Severity:       SeverityCritical,
			Title:          "Sequential Scan",
			Details:        "No indexed filters were detected for this query.",
			Recommendation: "Add a selective WHERE clause or an index that supports the query filters.",
			Table:          ctx.Table.Name,
		}}
	}
	if hasSupportingIndex(ctx.Table, effective) {
		return nil
	}
	return []Finding{{
		Code:           "sequential_scan",
		Rule:           "Sequential Scan",
		Severity:       SeverityWarning,
		Title:          "Sequential Scan",
		Details:        fmt.Sprintf("The query filters on %s, but no supporting index was found.", strings.Join(effective, ", ")),
		Recommendation: recommendationForColumns(effective),
		Table:          ctx.Table.Name,
		Columns:        append([]string(nil), effective...),
	}}
}

type RuleMissingIndex struct{}

func (RuleMissingIndex) Name() string { return "missing_index" }

func (RuleMissingIndex) Apply(ctx Context) []Finding {
	effective := effectiveFilterColumns(ctx)
	if len(effective) != 1 {
		return nil
	}
	if hasSupportingIndex(ctx.Table, effective) {
		return nil
	}
	return []Finding{{
		Code:           "missing_index",
		Rule:           "Missing Index",
		Severity:       SeverityWarning,
		Title:          "Missing Index",
		Details:        fmt.Sprintf("The query filters on %s without an index.", effective[0]),
		Recommendation: fmt.Sprintf("Create an index on (%s).", effective[0]),
		Table:          ctx.Table.Name,
		Columns:        append([]string(nil), effective...),
	}}
}

type RuleMissingCompositeIndex struct{}

func (RuleMissingCompositeIndex) Name() string { return "missing_composite_index" }

func (RuleMissingCompositeIndex) Apply(ctx Context) []Finding {
	effective := effectiveFilterColumns(ctx)
	if len(effective) < 2 {
		return nil
	}
	if hasSupportingIndex(ctx.Table, effective) {
		return nil
	}
	return []Finding{{
		Code:           "missing_composite_index",
		Rule:           "Missing Composite Index",
		Severity:       SeverityWarning,
		Title:          "Missing Composite Index",
		Details:        fmt.Sprintf("The query filters on %s, but no composite index matches the filter set.", strings.Join(effective, ", ")),
		Recommendation: fmt.Sprintf("Create a composite index on (%s).", strings.Join(effective, ", ")),
		Table:          ctx.Table.Name,
		Columns:        append([]string(nil), effective...),
	}}
}

type RuleLargeOffset struct{}

func (RuleLargeOffset) Name() string { return "large_offset" }

func (RuleLargeOffset) Apply(ctx Context) []Finding {
	if !ctx.Query.HasOffset || ctx.Query.Offset <= ctx.LargeOffsetThresh {
		return nil
	}
	return []Finding{{
		Code:           "large_offset",
		Rule:           "Large OFFSET",
		Severity:       SeverityWarning,
		Title:          "Large OFFSET",
		Details:        fmt.Sprintf("The query uses OFFSET %d, which becomes expensive for large pages.", ctx.Query.Offset),
		Recommendation: "Prefer keyset pagination or reduce the offset depth.",
		Table:          ctx.Table.Name,
	}}
}

type RulePotentialFullTableScan struct{}

func (RulePotentialFullTableScan) Name() string { return "potential_full_table_scan" }

func (RulePotentialFullTableScan) Apply(ctx Context) []Finding {
	if ctx.Query.Kind != QueryKindSelect && ctx.Query.Kind != QueryKindUpdate && ctx.Query.Kind != QueryKindDelete {
		return nil
	}
	if ctx.Query.HasWhere {
		effective := effectiveFilterColumns(ctx)
		if len(effective) > 0 && hasSupportingIndex(ctx.Table, effective) {
			return nil
		}
		return []Finding{{
			Code:           "potential_full_table_scan",
			Rule:           "Potential Full Table Scan",
			Severity:       SeverityWarning,
			Title:          "Potential Full Table Scan",
			Details:        "The filter set does not appear to be backed by a useful index.",
			Recommendation: recommendationForColumns(effective),
			Table:          ctx.Table.Name,
			Columns:        append([]string(nil), effective...),
		}}
	}
	return []Finding{{
		Code:           "potential_full_table_scan",
		Rule:           "Potential Full Table Scan",
		Severity:       SeverityCritical,
		Title:          "Potential Full Table Scan",
		Details:        "The query can scan the full table because it has no WHERE clause.",
		Recommendation: "Add a WHERE clause or a supporting index to reduce scanned rows.",
		Table:          ctx.Table.Name,
	}}
}

type RuleInefficientOrderBy struct{}

func (RuleInefficientOrderBy) Name() string { return "inefficient_order_by" }

func (RuleInefficientOrderBy) Apply(ctx Context) []Finding {
	if len(ctx.Query.OrderByCols) == 0 {
		return nil
	}
	candidate := append([]string{}, effectiveFilterColumns(ctx)...)
	candidate = append(candidate, ctx.Query.OrderByCols...)
	if hasSupportingIndex(ctx.Table, candidate) {
		return nil
	}
	if ctx.Query.HasOffset || ctx.Query.Kind == QueryKindSelect {
		return []Finding{{
			Code:           "inefficient_order_by",
			Rule:           "Inefficient ORDER BY",
			Severity:       SeverityWarning,
			Title:          "Inefficient ORDER BY",
			Details:        fmt.Sprintf("ORDER BY %s is not supported by an index.", strings.Join(ctx.Query.OrderByCols, ", ")),
			Recommendation: fmt.Sprintf("Create an index that matches the filter and sort columns: (%s).", strings.Join(candidate, ", ")),
			Table:          ctx.Table.Name,
			Columns:        append([]string(nil), candidate...),
		}}
	}
	return nil
}

func effectiveFilterColumns(ctx Context) []string {
	if ctx.Table == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, col := range policyColumns(ctx.Table) {
		key := strings.ToLower(col)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, col)
	}
	for _, col := range ctx.Query.WhereColumns {
		key := strings.ToLower(col)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, col)
	}
	return out
}

func policyColumns(table *schema.Table) []string {
	if table == nil {
		return nil
	}
	var cols []string
	for _, col := range table.Columns {
		if col == nil {
			continue
		}
		if col.Scope != schema.ScopeNone || col.SoftDelete {
			cols = append(cols, col.Name)
		}
	}
	return cols
}

func hasSupportingIndex(table *schema.Table, columns []string) bool {
	if table == nil || len(columns) == 0 {
		return false
	}
	want := normalizeColumns(columns)
	for _, idx := range table.Indexes {
		if idx == nil {
			continue
		}
		if prefixMatches(idx.Columns, want) {
			return true
		}
	}
	for _, c := range table.Constraints {
		if c == nil {
			continue
		}
		if c.Kind != schema.ConstraintPrimaryKey && c.Kind != schema.ConstraintUnique {
			continue
		}
		if prefixMatches(c.Columns, want) {
			return true
		}
	}
	for _, col := range table.Columns {
		if col == nil {
			continue
		}
		if len(want) == 1 && strings.EqualFold(col.Name, want[0]) && (col.PrimaryKey || col.Unique) {
			return true
		}
	}
	return false
}

func prefixMatches(indexCols, want []string) bool {
	if len(indexCols) < len(want) {
		return false
	}
	for i := range want {
		if !strings.EqualFold(indexCols[i], want[i]) {
			return false
		}
	}
	return true
}

func normalizeColumns(cols []string) []string {
	out := make([]string, 0, len(cols))
	for _, col := range cols {
		col = strings.TrimSpace(strings.Trim(col, `"`))
		if col == "" {
			continue
		}
		if idx := strings.LastIndex(col, "."); idx >= 0 {
			col = col[idx+1:]
		}
		out = append(out, strings.ToLower(col))
	}
	return out
}

func recommendationForColumns(cols []string) string {
	if len(cols) == 0 {
		return "Create an index that matches the query predicates."
	}
	if len(cols) == 1 {
		return fmt.Sprintf("Create an index on (%s).", cols[0])
	}
	return fmt.Sprintf("Create a composite index on (%s).", strings.Join(cols, ", "))
}

func findTable(s *schema.Schema, name string) *schema.Table {
	if s == nil {
		return nil
	}
	name = strings.ToLower(strings.Trim(name, `"`))
	for _, table := range s.Tables {
		if table != nil && strings.EqualFold(table.Name, name) {
			return table
		}
	}
	return nil
}

func extractTable(sqlText, expr string) string {
	re := regexp.MustCompile(expr)
	matches := re.FindStringSubmatch(sqlText)
	if len(matches) < 2 {
		return ""
	}
	return strings.Trim(matches[1], `"`)
}

func extractClause(sqlText, keyword string) (string, bool) {
	lower := strings.ToLower(sqlText)
	key := strings.ToLower(keyword)
	start := strings.Index(lower, key)
	if start < 0 {
		return "", false
	}
	start += len(key)
	segment := sqlText[start:]
	end := len(segment)
	for _, marker := range []string{" group by ", " order by ", " having ", " limit ", " offset "} {
		if idx := strings.Index(strings.ToLower(segment), marker); idx >= 0 && idx < end {
			end = idx
		}
	}
	return strings.TrimSpace(segment[:end]), true
}

func extractOffset(sqlText string) (int, bool) {
	re := regexp.MustCompile(`(?is)\boffset\s+(\d+)`)
	matches := re.FindStringSubmatch(sqlText)
	if len(matches) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func extractWhereColumns(where string) []string {
	if where == "" {
		return nil
	}
	re := regexp.MustCompile(`(?is)(?:^|[^\w.])(?:"?([a-z_][a-z0-9_]*)"?(?:\."?([a-z_][a-z0-9_]*)"?)?)\s*(=|<>|!=|>=|<=|>|<|\bin\b|\blike\b|\bilike\b|\bis\s+null\b|\bis\s+not\s+null\b|\bbetween\b)`)
	matches := re.FindAllStringSubmatch(where, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		col := m[1]
		if m[2] != "" {
			col = m[2]
		}
		if isSQLKeyword(col) {
			continue
		}
		out = append(out, strings.ToLower(col))
	}
	return uniqueStrings(out)
}

func extractOrderByColumns(orderBy string) []string {
	if orderBy == "" {
		return nil
	}
	parts := strings.Split(orderBy, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		col := fields[0]
		col = strings.Trim(col, `"`)
		if idx := strings.LastIndex(col, "."); idx >= 0 {
			col = col[idx+1:]
		}
		if strings.HasSuffix(strings.ToLower(col), "()") {
			continue
		}
		if isSQLKeyword(col) {
			continue
		}
		out = append(out, strings.ToLower(col))
	}
	return uniqueStrings(out)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		key := strings.ToLower(strings.TrimSpace(v))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func isSQLKeyword(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "and", "or", "where", "on", "in", "as", "is", "not", "null", "like", "ilike", "between", "exists":
		return true
	default:
		return false
	}
}

func severityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

func errorf(format string, args ...any) error {
	return errkind.New(errkind.KindConfiguration, fmt.Sprintf("query analyzer: "+format, args...))
}
