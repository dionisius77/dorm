package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"dorm/access"
	"dorm/dialect"
	"dorm/schema"
)

var ErrNotFound = errors.New("orm: not found")

type Logger interface {
	Printf(string, ...any)
}

type Config struct {
	DB                  *sql.DB
	Tx                  *sql.Tx
	Dialect             dialect.Dialect
	Schema              *schema.Schema
	Logger              Logger
	Observability       ObservabilityConfig
	Access              access.Engine
	PrepareStatements   bool
	SoftDeleteByDefault bool
}

type DB struct {
	db                  *sql.DB
	tx                  *sql.Tx
	dialect             dialect.Dialect
	schema              *schema.Schema
	logger              Logger
	observability       ObservabilityConfig
	access              access.Engine
	prepareStatements   bool
	softDeleteByDefault bool
	stmtMu              sync.Mutex
	stmts               map[string]*sql.Stmt
}

type Session struct {
	db          *DB
	ctx         context.Context
	withDeleted bool
}

func (s *Session) resolveModel(model any) (*tableMeta, reflect.Value, error) {
	return s.db.resolveModel(model)
}

func New(cfg Config) *DB {
	obs := cfg.Observability.Normalized()
	return &DB{
		db:                  cfg.DB,
		tx:                  cfg.Tx,
		dialect:             cfg.Dialect,
		schema:              cfg.Schema,
		logger:              cfg.Logger,
		observability:       obs,
		access:              cfg.Access,
		prepareStatements:   cfg.PrepareStatements,
		softDeleteByDefault: cfg.SoftDeleteByDefault,
		stmts:               map[string]*sql.Stmt{},
	}
}

func (db *DB) WithContext(ctx context.Context) *Session {
	return &Session{db: db, ctx: ctx}
}

func (s *Session) WithDeleted() *Session {
	cp := *s
	cp.withDeleted = true
	return &cp
}

func (s *Session) Find(dest any, opts ...QueryOption) error {
	state, err := s.buildState(dest, opts...)
	if err != nil {
		return err
	}
	sqlText, args, err := s.db.buildSelectSQL(s.ctx, state)
	if err != nil {
		return err
	}
	rows, err := s.db.queryContext(s.ctx, sqlText, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanIntoSlice(rows, dest, state.table)
}

func (s *Session) FindOne(dest any, opts ...QueryOption) error {
	opts = append(opts, Limit(1))
	if err := s.Find(dest, opts...); err != nil {
		return err
	}
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Slice || rv.Elem().Len() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Session) Create(model any) error {
	meta, rv, err := s.resolveModel(model)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	table := meta.table
	values := map[string]any{}
	if err := populateAuditAndScope(s.ctx, table, rv, values, now, access.OpInsert); err != nil {
		return err
	}
	if err := applyStructFieldValues(table, rv, values, now, true); err != nil {
		return err
	}
	cols, args := orderedColumnsAndArgs(table, values, true)
	if len(cols) == 0 {
		return fmt.Errorf("orm: no insertable columns for %s", table.Name)
	}
	sqlText, err := s.db.dialect.RenderInsert(table.Name, cols, returningColumns(table))
	if err != nil {
		return err
	}
	return s.db.execReturning(s.ctx, sqlText, args, model, table)
}

func (s *Session) Update(model any) error {
	meta, rv, err := s.resolveModel(model)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	table := meta.table
	values := map[string]any{}
	if err := populateAuditAndScope(s.ctx, table, rv, values, now, access.OpUpdate); err != nil {
		return err
	}
	if err := applyStructFieldValues(table, rv, values, now, false); err != nil {
		return err
	}
	pkCols, pkArgs, err := primaryKeyValues(table, rv)
	if err != nil {
		return err
	}
	if len(pkCols) == 0 {
		return fmt.Errorf("orm: update requires primary key")
	}
	set, args := updateSetClauses(table, values, pkCols)
	whereSQL, whereArgs := buildWhereClauses(pkCols, pkArgs, accessPredicates(s.ctx, table))
	sqlText, err := s.db.dialect.RenderUpdate(table.Name, set, whereSQL, returningColumns(table))
	if err != nil {
		return err
	}
	return s.db.execReturning(s.ctx, sqlText, append(args, whereArgs...), model, table)
}

func (s *Session) Delete(model any) error {
	meta, rv, err := s.resolveModel(model)
	if err != nil {
		return err
	}
	table := meta.table
	if soft := softDeleteColumn(table); soft != nil {
		return s.softDelete(model, table, rv, soft)
	}
	pkCols, pkArgs, err := primaryKeyValues(table, rv)
	if err != nil {
		return err
	}
	if len(pkCols) == 0 {
		return fmt.Errorf("orm: delete requires primary key")
	}
	whereSQL, whereArgs := buildWhereClauses(pkCols, pkArgs, accessPredicates(s.ctx, table))
	sqlText, err := s.db.dialect.RenderDelete(table.Name, whereSQL, nil)
	if err != nil {
		return err
	}
	_, err = s.db.execContext(s.ctx, sqlText, whereArgs...)
	return err
}

func (s *Session) SoftDelete(model any) error {
	meta, rv, err := s.resolveModel(model)
	if err != nil {
		return err
	}
	table := meta.table
	soft := softDeleteColumn(table)
	if soft == nil {
		return fmt.Errorf("orm: table %s has no soft delete column", table.Name)
	}
	return s.softDelete(model, table, rv, soft)
}

func (s *Session) Upsert(model any) error {
	meta, rv, err := s.resolveModel(model)
	if err != nil {
		return err
	}
	table := meta.table
	values := map[string]any{}
	if err := applyStructFieldValues(table, rv, values, time.Now().UTC(), true); err != nil {
		return err
	}
	if err := populateAuditAndScope(s.ctx, table, rv, values, time.Now().UTC(), access.OpInsert); err != nil {
		return err
	}
	cols, args := orderedColumnsAndArgs(table, values, true)
	if len(cols) == 0 {
		return fmt.Errorf("orm: no upsertable columns for %s", table.Name)
	}
	conflict := conflictColumns(table)
	if len(conflict) == 0 {
		return fmt.Errorf("orm: upsert requires primary key or unique constraint")
	}
	insertSQL, err := s.db.dialect.RenderInsert(table.Name, cols, returningColumns(table))
	if err != nil {
		return err
	}
	set := upsertSetClauses(table, values, conflict)
	sqlText := strings.TrimSuffix(insertSQL, ";") + " ON CONFLICT (" + strings.Join(quoteColumns(conflict, s.db.dialect), ", ") + ") DO UPDATE SET " + strings.Join(set, ", ") + ";"
	return s.db.execReturning(s.ctx, sqlText, args, model, table)
}

func (s *Session) Tx(fn func(*Session) error) error {
	return s.db.Tx(s.ctx, fn)
}

func (db *DB) Tx(ctx context.Context, fn func(*Session) error) error {
	if db.tx != nil {
		return fn(&Session{db: db, ctx: ctx})
	}
	if db.db == nil {
		return fmt.Errorf("orm: nil db")
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	child := *db
	child.tx = tx
	child.db = nil
	child.stmts = map[string]*sql.Stmt{}
	s := &Session{db: &child, ctx: ctx}
	if err := fn(s); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type QueryOption func(*queryState)

func Where(expr string, args ...any) QueryOption {
	return func(q *queryState) {
		q.where = append(q.where, predicate{expr: expr, args: args})
	}
}

func OrderBy(expr string) QueryOption {
	return func(q *queryState) {
		q.orderBy = append(q.orderBy, expr)
	}
}

func Limit(n int) QueryOption {
	return func(q *queryState) {
		q.limit = &n
	}
}

func Offset(n int) QueryOption {
	return func(q *queryState) {
		q.offset = &n
	}
}

type queryState struct {
	table       *schema.Table
	columns     []string
	where       []predicate
	orderBy     []string
	limit       *int
	offset      *int
	withDeleted bool
}

type predicate struct {
	expr string
	args []any
}

func (s *Session) buildState(dest any, opts ...QueryOption) (*queryState, error) {
	meta, err := s.db.resolveDest(dest)
	if err != nil {
		return nil, err
	}
	state := &queryState{table: meta.table, columns: columnsForTable(meta.table)}
	if s.withDeleted {
		state.withDeleted = true
	}
	for _, opt := range opts {
		opt(state)
	}
	if !state.withDeleted && softDeleteColumn(meta.table) != nil {
		state.where = append(state.where, predicate{expr: softDeleteColumn(meta.table).Name + " IS NULL"})
	}
	preds, _, err := s.db.access.Apply(s.ctx, meta.table, access.OpQuery, nil)
	if err != nil {
		return nil, err
	}
	for _, pred := range preds {
		state.where = append(state.where, predicate{expr: pred.SQL, args: pred.Args})
	}
	return state, nil
}

func (db *DB) buildSelectSQL(ctx context.Context, state *queryState) (string, []any, error) {
	var clauses []string
	var args []any
	for _, p := range state.where {
		clause, clauseArgs := bindClause(p.expr, p.args, len(args)+1)
		clauses = append(clauses, clause)
		args = append(args, clauseArgs...)
	}
	sqlText, err := db.dialect.RenderSelect(state.table.Name, quoteColumns(state.columns, db.dialect), clauses, state.orderBy, state.limit, state.offset)
	if err != nil {
		return "", nil, err
	}
	return sqlText, args, nil
}

func (db *DB) resolveDest(dest any) (*tableMeta, error) {
	if dest == nil {
		return nil, fmt.Errorf("orm: nil destination")
	}
	rt := reflect.TypeOf(dest)
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Slice {
		return nil, fmt.Errorf("orm: destination must be pointer to slice")
	}
	elem := rt.Elem()
	if elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() != reflect.Struct {
		return nil, fmt.Errorf("orm: slice element must be struct")
	}
	table := db.lookupTableForType(elem)
	return &tableMeta{table: table, typ: elem}, nil
}

type tableMeta struct {
	table *schema.Table
	typ   reflect.Type
}

func (db *DB) lookupTableForType(t reflect.Type) *schema.Table {
	if db.schema != nil {
		for _, table := range db.schema.Tables {
			if table == nil {
				continue
			}
			if table.GoTypeName == t.Name() {
				return table
			}
		}
	}
	return &schema.Table{
		Name:        schema.Pluralize(schema.ToSnakeCase(t.Name())),
		GoTypeName:  t.Name(),
		PackageName: t.PkgPath(),
		Columns:     columnsFromType(t),
	}
}

func (db *DB) resolveModel(model any) (*tableMeta, reflect.Value, error) {
	rv := reflect.ValueOf(model)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil, reflect.Value{}, fmt.Errorf("orm: model must be non-nil pointer")
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return nil, reflect.Value{}, fmt.Errorf("orm: model must point to struct")
	}
	return &tableMeta{table: db.lookupTableForType(elem.Type()), typ: elem.Type()}, elem, nil
}

func columnsFromType(t reflect.Type) []*schema.Column {
	var cols []*schema.Column
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		col := &schema.Column{
			Name:   schema.ToSnakeCase(f.Name),
			GoName: f.Name,
			Type:   reflectTypeToSchemaType(f.Type),
		}
		if f.Name == "ID" {
			col.PrimaryKey = true
			col.Unique = true
		}
		switch f.Name {
		case "CreatedAt":
			col.CreatedAt = true
		case "UpdatedAt":
			col.UpdatedAt = true
		case "DeletedAt":
			col.DeletedAt = true
			col.Nullable = true
			col.SoftDelete = true
		case "CreatedBy":
			col.CreatedBy = true
		case "UpdatedBy":
			col.UpdatedBy = true
		case "DeletedBy":
			col.DeletedBy = true
		}
		cols = append(cols, col)
	}
	return cols
}

func reflectTypeToSchemaType(t reflect.Type) schema.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return schema.Type{Name: "boolean", Kind: schema.TypeBool}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return schema.Type{Name: "bigint", Kind: schema.TypeInt}
	case reflect.Float32, reflect.Float64:
		return schema.Type{Name: "double precision", Kind: schema.TypeFloat}
	case reflect.String:
		return schema.Type{Name: "text", Kind: schema.TypeString}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return schema.Type{Name: "bytea", Kind: schema.TypeBytes}
		}
		elem := reflectTypeToSchemaType(t.Elem())
		return schema.Type{Name: elem.Name + "[]", Kind: schema.TypeArray, ArrayOf: &elem}
	case reflect.Struct:
		if t.PkgPath() == "time" && t.Name() == "Time" {
			return schema.Type{Name: "timestamptz", Kind: schema.TypeTime}
		}
		if strings.EqualFold(t.Name(), "UUID") {
			return schema.Type{Name: "uuid", Kind: schema.TypeUUID}
		}
		return schema.Type{Name: schema.ToSnakeCase(t.Name()), Kind: schema.TypeCustom}
	default:
		return schema.Type{Name: "text", Kind: schema.TypeUnknown}
	}
}

func scanIntoSlice(rows *sql.Rows, dest any, table *schema.Table) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("orm: destination must be pointer to slice")
	}
	slice := rv.Elem()
	elemType := slice.Type().Elem()
	isPtr := false
	if elemType.Kind() == reflect.Pointer {
		isPtr = true
		elemType = elemType.Elem()
	}
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	fieldMap := structFieldMap(elemType)
	for rows.Next() {
		holder := make([]any, len(cols))
		for i := range holder {
			var value any
			holder[i] = &value
		}
		if err := rows.Scan(holder...); err != nil {
			return err
		}
		item := reflect.New(elemType).Elem()
		for i, col := range cols {
			f, ok := fieldMap[strings.ToLower(col)]
			if !ok || !f.CanSet() {
				continue
			}
			if valuePtr, ok := holder[i].(*any); ok {
				assignReflectValue(f, *valuePtr)
			}
		}
		if isPtr {
			slice = reflect.Append(slice, item.Addr())
		} else {
			slice = reflect.Append(slice, item)
		}
	}
	rv.Elem().Set(slice)
	return rows.Err()
}

func structFieldMap(t reflect.Type) map[string]reflect.Value {
	out := map[string]reflect.Value{}
	v := reflect.New(t).Elem()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		out[strings.ToLower(schema.ToSnakeCase(f.Name))] = v.Field(i)
	}
	return out
}

func assignReflectValue(dst reflect.Value, value any) {
	if !dst.CanSet() || value == nil {
		return
	}
	v := reflect.ValueOf(value)
	if v.Type().AssignableTo(dst.Type()) {
		dst.Set(v)
		return
	}
	if dst.Kind() == reflect.Pointer {
		elem := reflect.New(dst.Type().Elem())
		assignReflectValue(elem.Elem(), value)
		dst.Set(elem)
		return
	}
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(fmt.Sprint(value))
	case reflect.Bool:
		if b, ok := value.(bool); ok {
			dst.SetBool(b)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		setInt(dst, value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		setUint(dst, value)
	case reflect.Float32, reflect.Float64:
		setFloat(dst, value)
	default:
		if v.Type().ConvertibleTo(dst.Type()) {
			dst.Set(v.Convert(dst.Type()))
		}
	}
}

func setInt(dst reflect.Value, value any) {
	switch v := value.(type) {
	case int64:
		dst.SetInt(v)
	case int:
		dst.SetInt(int64(v))
	case float64:
		dst.SetInt(int64(v))
	}
}

func setUint(dst reflect.Value, value any) {
	switch v := value.(type) {
	case int64:
		dst.SetUint(uint64(v))
	case int:
		dst.SetUint(uint64(v))
	case uint64:
		dst.SetUint(v)
	}
}

func setFloat(dst reflect.Value, value any) {
	switch v := value.(type) {
	case float64:
		dst.SetFloat(v)
	case float32:
		dst.SetFloat(float64(v))
	case int64:
		dst.SetFloat(float64(v))
	}
}

func populateAuditAndScope(ctx context.Context, table *schema.Table, rv reflect.Value, values map[string]any, now time.Time, op access.Operation) error {
	ac, _ := access.FromContext(ctx)
	preds, writes, err := access.NewEngine().Apply(ctx, table, op, nil)
	_ = preds
	if err != nil {
		return err
	}
	for _, w := range writes {
		values[w.Field] = w.Value
		setFieldByColumn(rv, table, w.Field, w.Value)
	}
	for _, col := range table.Columns {
		switch {
		case col.CreatedAt && op == access.OpInsert:
			values[col.Name] = now
			setFieldByColumn(rv, table, col.Name, now)
		case col.UpdatedAt:
			values[col.Name] = now
			setFieldByColumn(rv, table, col.Name, now)
		case col.DeletedAt && op == access.OpDelete:
			values[col.Name] = now
			setFieldByColumn(rv, table, col.Name, now)
		}
	}
	_ = ac
	return nil
}

func applyStructFieldValues(table *schema.Table, rv reflect.Value, values map[string]any, now time.Time, isInsert bool) error {
	for _, col := range table.Columns {
		if _, ok := values[col.Name]; ok {
			continue
		}
		field := fieldByColumn(rv, table, col.Name)
		if !field.IsValid() || !field.CanInterface() {
			continue
		}
		v := field.Interface()
		if isZeroValue(v) {
			switch {
			case col.CreatedAt && isInsert:
				values[col.Name] = now
				setFieldByColumn(rv, table, col.Name, now)
			case col.UpdatedAt:
				values[col.Name] = now
				setFieldByColumn(rv, table, col.Name, now)
			case col.DeletedAt && !isInsert:
				values[col.Name] = now
				setFieldByColumn(rv, table, col.Name, now)
			case col.CreatedBy || col.UpdatedBy || col.DeletedBy || col.Scope != schema.ScopeNone:
				// already handled by access layer if context exists
			default:
				values[col.Name] = v
			}
			continue
		}
		values[col.Name] = v
	}
	return nil
}

func orderedColumnsAndArgs(table *schema.Table, values map[string]any, includePrimary bool) ([]string, []any) {
	var cols []string
	var args []any
	for _, col := range table.Columns {
		if !includePrimary && col.PrimaryKey {
			continue
		}
		v, ok := values[col.Name]
		if !ok {
			continue
		}
		cols = append(cols, col.Name)
		args = append(args, v)
	}
	return cols, args
}

func returningColumns(table *schema.Table) []string {
	var cols []string
	for _, col := range table.Columns {
		if col.PrimaryKey || col.CreatedAt || col.UpdatedAt || col.DeletedAt {
			cols = append(cols, col.Name)
		}
	}
	return cols
}

func updateSetClauses(table *schema.Table, values map[string]any, pkCols []string) ([]string, []any) {
	pk := map[string]struct{}{}
	for _, name := range pkCols {
		pk[name] = struct{}{}
	}
	var set []string
	var args []any
	argIndex := 1
	for _, col := range table.Columns {
		if _, ok := pk[col.Name]; ok {
			continue
		}
		v, ok := values[col.Name]
		if !ok {
			continue
		}
		set = append(set, fmt.Sprintf("%s = %s", quoteIdent(col.Name), placeholder(argIndex)))
		args = append(args, v)
		argIndex++
	}
	return set, args
}

func upsertSetClauses(table *schema.Table, values map[string]any, conflict []string) []string {
	conflictSet := map[string]struct{}{}
	for _, name := range conflict {
		conflictSet[name] = struct{}{}
	}
	var set []string
	for _, col := range table.Columns {
		if _, ok := conflictSet[col.Name]; ok {
			continue
		}
		if _, ok := values[col.Name]; !ok {
			continue
		}
		set = append(set, fmt.Sprintf("%s = EXCLUDED.%s", quoteIdent(col.Name), quoteIdent(col.Name)))
	}
	return set
}

func conflictColumns(table *schema.Table) []string {
	var cols []string
	for _, col := range table.Columns {
		if col.PrimaryKey || col.Unique {
			cols = append(cols, col.Name)
		}
	}
	return cols
}

func buildWhereClauses(cols []string, args []any, extra []predicate) ([]string, []any) {
	var where []string
	whereArgs := append([]any(nil), args...)
	for _, col := range cols {
		where = append(where, quoteIdent(col)+" = "+placeholder(len(whereArgs)+1))
	}
	for _, p := range extra {
		clause, clauseArgs := bindClause(p.expr, p.args, len(whereArgs)+1)
		where = append(where, clause)
		whereArgs = append(whereArgs, clauseArgs...)
	}
	return where, whereArgs
}

func accessPredicates(ctx context.Context, table *schema.Table) []predicate {
	preds, _, _ := access.NewEngine().Apply(ctx, table, access.OpQuery, nil)
	var clauses []predicate
	for _, pred := range preds {
		clauses = append(clauses, predicate{expr: pred.SQL, args: pred.Args})
	}
	return clauses
}

func softDeleteColumn(table *schema.Table) *schema.Column {
	return access.SoftDeleteColumn(table)
}

func (s *Session) softDelete(model any, table *schema.Table, rv reflect.Value, soft *schema.Column) error {
	now := time.Now().UTC()
	values := map[string]any{soft.Name: now}
	setFieldByColumn(rv, table, soft.Name, now)
	pkCols, pkArgs, err := primaryKeyValues(table, rv)
	if err != nil {
		return err
	}
	whereSQL, whereArgs := buildWhereClauses(pkCols, pkArgs, accessPredicates(s.ctx, table))
	set := []string{quoteIdent(soft.Name) + " = " + placeholder(1)}
	sqlText, err := s.db.dialect.RenderUpdate(table.Name, set, whereSQL, nil)
	if err != nil {
		return err
	}
	_, err = s.db.execContext(s.ctx, sqlText, append([]any{now}, whereArgs...)...)
	_ = values
	return err
}

func primaryKeyValues(table *schema.Table, rv reflect.Value) ([]string, []any, error) {
	var cols []string
	var args []any
	for _, col := range table.Columns {
		if !col.PrimaryKey {
			continue
		}
		field := fieldByColumn(rv, table, col.Name)
		if !field.IsValid() {
			return nil, nil, fmt.Errorf("orm: missing primary key field %s", col.Name)
		}
		cols = append(cols, col.Name)
		args = append(args, field.Interface())
	}
	return cols, args, nil
}

func fieldByColumn(rv reflect.Value, table *schema.Table, col string) reflect.Value {
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Type().Field(i)
		if schema.ToSnakeCase(f.Name) == col {
			return rv.Field(i)
		}
	}
	return reflect.Value{}
}

func setFieldByColumn(rv reflect.Value, table *schema.Table, col string, value any) {
	field := fieldByColumn(rv, table, col)
	if !field.IsValid() || !field.CanSet() {
		return
	}
	assignReflectValue(field, value)
}

func quoteColumns(cols []string, d dialect.Dialect) []string {
	out := make([]string, 0, len(cols))
	for _, col := range cols {
		out = append(out, d.QuoteIdent(col))
	}
	return out
}

func columnsForTable(table *schema.Table) []string {
	if table == nil {
		return nil
	}
	cols := make([]string, 0, len(table.Columns))
	for _, col := range table.Columns {
		cols = append(cols, col.Name)
	}
	return cols
}

func bindClause(expr string, args []any, start int) (string, []any) {
	if expr == "" {
		return "", nil
	}
	if len(args) == 0 {
		return expr, nil
	}
	var b strings.Builder
	next := start
	for _, r := range expr {
		if r == '?' {
			b.WriteString("$")
			b.WriteString(fmt.Sprint(next))
			next++
			continue
		}
		b.WriteRune(r)
	}
	return b.String(), append([]any(nil), args...)
}

func placeholder(i int) string {
	return "$" + fmt.Sprint(i)
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func isZeroValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map:
		return rv.IsNil()
	default:
		return rv.IsZero()
	}
}

func (db *DB) execContext(ctx context.Context, sqlText string, args ...any) (sql.Result, error) {
	if db.tx != nil {
		return db.tx.ExecContext(ctx, sqlText, args...)
	}
	if db.db == nil {
		return nil, fmt.Errorf("orm: nil db")
	}
	if stmt, ok := db.prepared(sqlText); ok {
		return stmt.ExecContext(ctx, args...)
	}
	return db.db.ExecContext(ctx, sqlText, args...)
}

func (db *DB) queryContext(ctx context.Context, sqlText string, args ...any) (*sql.Rows, error) {
	if db.tx != nil {
		return db.tx.QueryContext(ctx, sqlText, args...)
	}
	if db.db == nil {
		return nil, fmt.Errorf("orm: nil db")
	}
	if stmt, ok := db.prepared(sqlText); ok {
		return stmt.QueryContext(ctx, args...)
	}
	return db.db.QueryContext(ctx, sqlText, args...)
}

func (db *DB) execReturning(ctx context.Context, sqlText string, args []any, model any, table *schema.Table) error {
	rows, err := db.queryContext(ctx, sqlText, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		cols, err := rows.Columns()
		if err != nil {
			return err
		}
		holder := make([]any, len(cols))
		for i := range holder {
			var value any
			holder[i] = &value
		}
		if err := rows.Scan(holder...); err != nil {
			return err
		}
		rv := reflect.ValueOf(model)
		if rv.Kind() == reflect.Pointer && !rv.IsNil() {
			for i, col := range cols {
				if valuePtr, ok := holder[i].(*any); ok {
					setFieldByColumn(rv, table, col, *valuePtr)
				}
			}
		}
	}
	return rows.Err()
}

func (db *DB) prepared(sqlText string) (*sql.Stmt, bool) {
	if !db.prepareStatements {
		return nil, false
	}
	db.stmtMu.Lock()
	defer db.stmtMu.Unlock()
	if stmt, ok := db.stmts[sqlText]; ok {
		return stmt, true
	}
	var stmt *sql.Stmt
	var err error
	if db.tx != nil {
		stmt, err = db.tx.PrepareContext(context.Background(), sqlText)
	} else if db.db != nil {
		stmt, err = db.db.PrepareContext(context.Background(), sqlText)
	}
	if err != nil {
		return nil, false
	}
	db.stmts[sqlText] = stmt
	return stmt, true
}
