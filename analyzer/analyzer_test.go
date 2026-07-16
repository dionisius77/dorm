package analyzer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/schema"
)

func TestAnalyzeMissingWhere(t *testing.T) {
	a := New(Config{})
	report := mustAnalyze(t, a, Input{
		SQL: "SELECT * FROM users",
		Schema: testAnalyzerSchema(&schema.Table{
			Name: "users",
			Columns: []*schema.Column{
				{Name: "id", PrimaryKey: true},
				{Name: "name"},
			},
		}),
	})
	ensureCodes(t, report, "missing_where", "sequential_scan", "potential_full_table_scan")
}

func TestAnalyzeMissingIndex(t *testing.T) {
	a := New(Config{})
	report := mustAnalyze(t, a, Input{
		SQL: "SELECT * FROM roles WHERE name = $1",
		Schema: testAnalyzerSchema(&schema.Table{
			Name: "roles",
			Columns: []*schema.Column{
				{Name: "id", PrimaryKey: true},
				{Name: "name"},
			},
		}),
	})
	ensureCodes(t, report, "missing_index", "sequential_scan", "potential_full_table_scan")
}

func TestAnalyzeMissingCompositeIndex(t *testing.T) {
	a := New(Config{})
	report := mustAnalyze(t, a, Input{
		SQL: "SELECT * FROM products WHERE sku = $1",
		Schema: testAnalyzerSchema(&schema.Table{
			Name: "products",
			Columns: []*schema.Column{
				{Name: "id", PrimaryKey: true},
				{Name: "company_id", Scope: schema.ScopeCompany},
				{Name: "deleted_at", SoftDelete: true},
				{Name: "sku", Unique: true},
				{Name: "name"},
			},
			Constraints: []*schema.Constraint{
				{Name: "products_sku_key", Kind: schema.ConstraintUnique, Columns: []string{"sku"}},
			},
		}),
	})
	ensureCodes(t, report, "missing_composite_index", "sequential_scan", "potential_full_table_scan")
}

func TestAnalyzeLargeOffset(t *testing.T) {
	a := New(Config{LargeOffsetThreshold: 1000})
	report := mustAnalyze(t, a, Input{
		SQL: "SELECT * FROM users WHERE name = $1 ORDER BY name ASC OFFSET 2001",
		Schema: testAnalyzerSchema(&schema.Table{
			Name: "users",
			Columns: []*schema.Column{
				{Name: "id", PrimaryKey: true},
				{Name: "name"},
			},
		}),
	})
	ensureCodes(t, report, "large_offset", "missing_index", "inefficient_order_by", "sequential_scan", "potential_full_table_scan")
}

func TestAnalyzeInefficientOrderBy(t *testing.T) {
	a := New(Config{})
	report := mustAnalyze(t, a, Input{
		SQL: "SELECT * FROM users WHERE company_id = $1 ORDER BY updated_at DESC",
		Schema: testAnalyzerSchema(&schema.Table{
			Name: "users",
			Columns: []*schema.Column{
				{Name: "id", PrimaryKey: true},
				{Name: "company_id", Scope: schema.ScopeCompany},
				{Name: "updated_at"},
			},
			Indexes: []*schema.Index{
				{Name: "users_company_id_idx", Columns: []string{"company_id"}},
			},
		}),
	})
	ensureCodes(t, report, "inefficient_order_by")
}

func TestAnalyzeAccessPolicyAware(t *testing.T) {
	a := New(Config{})
	report := mustAnalyze(t, a, Input{
		SQL: "SELECT * FROM products WHERE sku = $1",
		Schema: testAnalyzerSchema(&schema.Table{
			Name: "products",
			Columns: []*schema.Column{
				{Name: "id", PrimaryKey: true},
				{Name: "company_id", Scope: schema.ScopeCompany},
				{Name: "deleted_at", SoftDelete: true},
				{Name: "sku", Unique: true},
				{Name: "name"},
			},
			Constraints: []*schema.Constraint{
				{Name: "products_sku_key", Kind: schema.ConstraintUnique, Columns: []string{"sku"}},
			},
		}),
	})
	finding := findFinding(report, "missing_composite_index")
	if finding == nil {
		t.Fatal("expected missing_composite_index finding")
	}
	got := strings.ToLower(finding.Recommendation)
	if !strings.Contains(got, "company_id") || !strings.Contains(got, "deleted_at") || !strings.Contains(got, "sku") {
		t.Fatalf("expected policy-aware recommendation, got %q", finding.Recommendation)
	}
}

func TestAnalyzeValidationErrors(t *testing.T) {
	a := New(Config{})
	_, err := a.Analyze(context.Background(), Input{Schema: &schema.Schema{Name: "public"}})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, errkind.ErrConfiguration) {
		t.Fatalf("expected configuration error, got %v", err)
	}
}

func mustAnalyze(t *testing.T, a *Analyzer, input Input) Report {
	t.Helper()
	report, err := a.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	return report
}

func ensureCodes(t *testing.T, report Report, want ...string) {
	t.Helper()
	got := reportCodes(report)
	for _, code := range want {
		if !containsString(got, code) {
			t.Fatalf("expected finding %q in %v", code, got)
		}
	}
}

func reportCodes(report Report) []string {
	out := make([]string, 0, len(report.Findings))
	for _, finding := range report.Findings {
		out = append(out, finding.Code)
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func findFinding(report Report, code string) *Finding {
	for i := range report.Findings {
		if report.Findings[i].Code == code {
			return &report.Findings[i]
		}
	}
	return nil
}

func testAnalyzerSchema(table *schema.Table) *schema.Schema {
	return &schema.Schema{
		Name:   "public",
		Tables: []*schema.Table{table},
	}
}
