package orm

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/dionisius77/dorm/dialect"
	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/schema"
)

type BeforeCreateHook interface {
	BeforeCreate(context.Context, *DB) error
}

type AfterCreateHook interface {
	AfterCreate(context.Context, *DB) error
}

type BeforeUpdateHook interface {
	BeforeUpdate(context.Context, *DB) error
}

type AfterUpdateHook interface {
	AfterUpdate(context.Context, *DB) error
}

type BeforeDeleteHook interface {
	BeforeDelete(context.Context, *DB) error
}

type AfterDeleteHook interface {
	AfterDelete(context.Context, *DB) error
}

type BeforeFindHook interface {
	BeforeFind(context.Context) error
}

type AfterFindHook interface {
	AfterFind(context.Context, *DB) error
}

func (s *Session) Load(dest any, relation string) error {
	if s == nil {
		return errkind.New(errkind.KindConfiguration, "orm: nil session")
	}
	if relation == "" {
		return errkind.New(errkind.KindConfiguration, "orm: empty relation name")
	}
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errkind.New(errkind.KindConfiguration, "orm: load destination must be a non-nil pointer")
	}
	return s.loadValue(rv.Elem(), relation)
}

func (s *Session) Preload(dest any, relations ...string) error {
	for _, relation := range relations {
		if err := s.Load(dest, relation); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) loadValue(rv reflect.Value, relation string) error {
	if !rv.IsValid() {
		return errkind.New(errkind.KindInvalidSchema, "orm: invalid relation destination")
	}
	switch rv.Kind() {
	case reflect.Struct:
		return s.loadStructRelation(rv, relation)
	case reflect.Slice:
		for i := 0; i < rv.Len(); i++ {
			item := rv.Index(i)
			if item.Kind() == reflect.Pointer {
				if item.IsNil() {
					continue
				}
				item = item.Elem()
			}
			if item.Kind() != reflect.Struct {
				return errkind.New(errkind.KindInvalidSchema, "orm: preload destination must contain structs")
			}
			if err := s.loadStructRelation(item, relation); err != nil {
				return err
			}
		}
		return nil
	default:
		return errkind.New(errkind.KindInvalidSchema, "orm: preload destination must be a struct or slice")
	}
}

func invokeBeforeFindHook(ctx context.Context, db *DB, model any) error {
	return invokeLifecycleHook(ctx, db, "BeforeFind", model, func(context.Context) error {
		if hook, ok := model.(BeforeFindHook); ok {
			return hook.BeforeFind(ctx)
		}
		return nil
	})
}

func beforeFindHookModel(dest any) any {
	rv := reflect.ValueOf(dest)
	if !rv.IsValid() {
		return dest
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return dest
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice {
		return dest
	}
	elem := rv.Type().Elem()
	if elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() != reflect.Struct {
		return dest
	}
	return reflect.New(elem).Interface()
}

func (s *Session) loadStructRelation(parent reflect.Value, relation string) error {
	field, fieldInfo, ok := structFieldByName(parent, relation)
	if !ok {
		return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("orm: relation %q not found", relation))
	}

	switch field.Kind() {
	case reflect.Struct, reflect.Pointer:
		return s.loadBelongsToRelation(parent, field, fieldInfo, relation)
	case reflect.Slice:
		return s.loadHasManyRelation(parent, field, fieldInfo, relation)
	default:
		return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("orm: relation %q is not loadable", relation))
	}
}

func (s *Session) loadBelongsToRelation(parent, relationField reflect.Value, relationType reflect.StructField, relation string) error {
	targetType := relationField.Type()
	if targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	if targetType.Kind() != reflect.Struct {
		return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("orm: relation %q is not a struct or pointer to struct", relation))
	}

	fkField, ok := structFieldByCandidates(parent, relation+"ID", relation+"Id", schema.ToSnakeCase(relation)+"_id")
	if !ok {
		return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("orm: parent model missing foreign key for relation %q", relation))
	}
	if isZeroValue(fkField.Interface()) {
		relationField.Set(reflect.Zero(relationField.Type()))
		return nil
	}

	targetTable := s.db.lookupTableForType(targetType)
	pkCols := primaryKeyColumnsForTable(targetTable)
	if len(pkCols) != 1 {
		return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("orm: relation %q requires a single-column primary key", relation))
	}

	where := []string{s.db.dialect.QuoteIdent(pkCols[0]) + " = " + s.db.dialect.Placeholder(1)}
	args := []any{fkField.Interface()}
	where, args = appendAccessWhere(where, args, accessPredicates(s.ctx, targetTable), s.db.dialect)
	sqlText, err := s.db.dialect.RenderSelect(targetTable.Name, quoteColumns(columnsForTable(targetTable), s.db.dialect), where, nil, nil, nil)
	if err != nil {
		return errkind.Wrap(errkind.KindUnsupportedFeature, "orm: render relation load", err)
	}
	targetSlice := reflect.New(reflect.SliceOf(targetType))
	rows, err := s.db.queryContext(s.ctx, sqlText, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if err := scanIntoSlice(rows, targetSlice.Interface(), targetTable); err != nil {
		return err
	}
	if err := invokeAfterFindHooks(s.ctx, s.db, targetSlice.Elem()); err != nil {
		return err
	}
	if targetSlice.Elem().Len() == 0 {
		relationField.Set(reflect.Zero(relationField.Type()))
		return nil
	}
	first := targetSlice.Elem().Index(0)
	if relationField.Kind() == reflect.Pointer {
		if first.Kind() == reflect.Pointer {
			relationField.Set(first)
		} else {
			ptr := reflect.New(first.Type())
			ptr.Elem().Set(first)
			relationField.Set(ptr)
		}
		return nil
	}
	if first.Kind() == reflect.Pointer {
		relationField.Set(first.Elem())
		return nil
	}
	relationField.Set(first)
	return nil
}

func (s *Session) loadHasManyRelation(parent, relationField reflect.Value, relationType reflect.StructField, relation string) error {
	elemType := relationField.Type().Elem()
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("orm: relation %q is not a slice of structs", relation))
	}

	parentTable := s.db.lookupTableForType(parent.Type())
	parentPKField, ok := primaryKeyField(parent, parentTable)
	if !ok {
		return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("orm: parent model missing primary key for relation %q", relation))
	}
	if isZeroValue(parentPKField.Interface()) {
		relationField.Set(reflect.MakeSlice(relationField.Type(), 0, 0))
		return nil
	}

	childTable := s.db.lookupTableForType(elemType)
	fkName := schema.ToSnakeCase(parentTable.GoTypeName) + "_id"
	where := []string{s.db.dialect.QuoteIdent(fkName) + " = " + s.db.dialect.Placeholder(1)}
	args := []any{parentPKField.Interface()}
	where, args = appendAccessWhere(where, args, accessPredicates(s.ctx, childTable), s.db.dialect)
	sqlText, err := s.db.dialect.RenderSelect(childTable.Name, quoteColumns(columnsForTable(childTable), s.db.dialect), where, nil, nil, nil)
	if err != nil {
		return errkind.Wrap(errkind.KindUnsupportedFeature, "orm: render relation load", err)
	}
	targetSlice := reflect.New(relationField.Type())
	rows, err := s.db.queryContext(s.ctx, sqlText, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if err := scanIntoSlice(rows, targetSlice.Interface(), childTable); err != nil {
		return err
	}
	if err := invokeAfterFindHooks(s.ctx, s.db, targetSlice.Elem()); err != nil {
		return err
	}
	relationField.Set(targetSlice.Elem())
	return nil
}

func invokeBeforeCreateHook(ctx context.Context, db *DB, model any) error {
	return invokeLifecycleHook(ctx, db, "BeforeCreate", model, func(ctx context.Context) error {
		if hook, ok := model.(BeforeCreateHook); ok {
			return hook.BeforeCreate(ctx, db)
		}
		return nil
	})
}

func invokeAfterCreateHook(ctx context.Context, db *DB, model any) error {
	return invokeLifecycleHook(ctx, db, "AfterCreate", model, func(ctx context.Context) error {
		if hook, ok := model.(AfterCreateHook); ok {
			return hook.AfterCreate(ctx, db)
		}
		return nil
	})
}

func invokeBeforeUpdateHook(ctx context.Context, db *DB, model any) error {
	return invokeLifecycleHook(ctx, db, "BeforeUpdate", model, func(ctx context.Context) error {
		if hook, ok := model.(BeforeUpdateHook); ok {
			return hook.BeforeUpdate(ctx, db)
		}
		return nil
	})
}

func invokeAfterUpdateHook(ctx context.Context, db *DB, model any) error {
	return invokeLifecycleHook(ctx, db, "AfterUpdate", model, func(ctx context.Context) error {
		if hook, ok := model.(AfterUpdateHook); ok {
			return hook.AfterUpdate(ctx, db)
		}
		return nil
	})
}

func invokeBeforeDeleteHook(ctx context.Context, db *DB, model any) error {
	return invokeLifecycleHook(ctx, db, "BeforeDelete", model, func(ctx context.Context) error {
		if hook, ok := model.(BeforeDeleteHook); ok {
			return hook.BeforeDelete(ctx, db)
		}
		return nil
	})
}

func invokeAfterDeleteHook(ctx context.Context, db *DB, model any) error {
	return invokeLifecycleHook(ctx, db, "AfterDelete", model, func(ctx context.Context) error {
		if hook, ok := model.(AfterDeleteHook); ok {
			return hook.AfterDelete(ctx, db)
		}
		return nil
	})
}

func invokeAfterFindHooks(ctx context.Context, db *DB, rv reflect.Value) error {
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Slice:
		for i := 0; i < rv.Len(); i++ {
			if err := invokeAfterFindHooks(ctx, db, rv.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return invokeAfterFindHooks(ctx, db, rv.Elem())
	case reflect.Struct:
		model := rv.Interface()
		if rv.CanAddr() {
			model = rv.Addr().Interface()
		}
		return invokeLifecycleHook(ctx, db, "AfterFind", model, func(ctx context.Context) error {
			if hook, ok := model.(AfterFindHook); ok {
				return hook.AfterFind(ctx, db)
			}
			return nil
		})
	default:
		return nil
	}
}

func invokeLifecycleHook(ctx context.Context, db *DB, hookName string, model any, fn func(context.Context) error) error {
	if fn == nil {
		return nil
	}
	if db == nil {
		return fn(ctx)
	}
	modelName := lifecycleHookModelName(model)
	if db.dryRun != nil {
		db.dryRun.recordHook(hookName, modelName)
	}
	spanName := "db.hook." + schema.ToSnakeCase(hookName)
	return db.traceWithSpan(ctx, spanName, []Attribute{
		{Key: "orm.hook", Value: hookName},
		{Key: "orm.model", Value: modelName},
	}, func(ctx context.Context, span Span) error {
		addHookEvent(span, hookName, modelName)
		if err := fn(ctx); err != nil {
			return fmt.Errorf("%s(%s): %w", hookName, modelName, err)
		}
		return nil
	})
}

func addHookEvent(span Span, hookName, modelName string) {
	if span == nil {
		return
	}
	if eventer, ok := span.(interface {
		AddEvent(string, ...Attribute)
	}); ok {
		eventer.AddEvent(hookName, Attribute{Key: "orm.hook", Value: hookName}, Attribute{Key: "orm.model", Value: modelName})
	}
}

func lifecycleHookModelName(model any) string {
	if model == nil {
		return ""
	}
	t := reflect.TypeOf(model)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() != "" {
		return t.Name()
	}
	return t.String()
}

func structFieldByName(rv reflect.Value, name string) (reflect.Value, reflect.StructField, bool) {
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return reflect.Value{}, reflect.StructField{}, false
		}
		rv = rv.Elem()
	}
	return structFieldByNameRecursive(rv, name)
}

func structFieldByCandidates(rv reflect.Value, candidates ...string) (reflect.Value, bool) {
	for _, candidate := range candidates {
		if field, _, ok := structFieldByName(rv, candidate); ok {
			return field, true
		}
	}
	return reflect.Value{}, false
}

func structFieldByNameRecursive(rv reflect.Value, name string) (reflect.Value, reflect.StructField, bool) {
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		value := rv.Field(i)
		if strings.EqualFold(field.Name, name) || strings.EqualFold(schema.ToSnakeCase(field.Name), schema.ToSnakeCase(name)) {
			return value, field, true
		}
		if !field.Anonymous {
			continue
		}
		next := value
		if next.Kind() == reflect.Pointer {
			if next.IsNil() {
				continue
			}
			next = next.Elem()
		}
		if next.Kind() != reflect.Struct {
			continue
		}
		if found, sf, ok := structFieldByNameRecursive(next, name); ok {
			return found, sf, true
		}
	}
	return reflect.Value{}, reflect.StructField{}, false
}

func primaryKeyColumnsForTable(table *schema.Table) []string {
	if table == nil {
		return nil
	}
	var cols []string
	for _, col := range table.Columns {
		if col != nil && col.PrimaryKey {
			cols = append(cols, col.Name)
		}
	}
	if len(cols) == 0 {
		for _, col := range table.Columns {
			if col != nil && col.Name == "id" {
				return []string{col.Name}
			}
		}
	}
	return cols
}

func primaryKeyField(rv reflect.Value, table *schema.Table) (reflect.Value, bool) {
	for _, col := range primaryKeyColumnsForTable(table) {
		field, _, ok := structFieldByName(rv, col)
		if ok && field.IsValid() {
			return field, true
		}
	}
	if field, ok := structFieldByCandidates(rv, "ID", "Id", "id"); ok {
		return field, true
	}
	return reflect.Value{}, false
}

func (s *Session) queryOneInto(target any, table *schema.Table, where []string, args ...any) error {
	where, args = appendAccessWhere(where, args, accessPredicates(s.ctx, table), s.db.dialect)
	sqlText, err := s.db.dialect.RenderSelect(table.Name, quoteColumns(columnsForTable(table), s.db.dialect), where, nil, nil, nil)
	if err != nil {
		return errkind.Wrap(errkind.KindUnsupportedFeature, "orm: render query", err)
	}
	rows, err := s.db.queryContext(s.ctx, sqlText, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanIntoSlice(rows, target, table)
}

func appendAccessWhere(where []string, args []any, extra []predicate, d dialect.Dialect) ([]string, []any) {
	if len(extra) == 0 {
		return where, args
	}
	clauses := make([]string, 0, len(extra))
	outArgs := append([]any(nil), args...)
	for _, p := range extra {
		clause, clauseArgs := bindClause(p.expr, p.args, len(outArgs)+1, d)
		clauses = append(clauses, clause)
		outArgs = append(outArgs, clauseArgs...)
	}
	return append(where, clauses...), outArgs
}

func ensurePointerToStructOrSlice(v reflect.Value) error {
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errkind.New(errkind.KindConfiguration, "orm: destination must be a non-nil pointer")
	}
	switch v.Elem().Kind() {
	case reflect.Struct, reflect.Slice:
		return nil
	default:
		return errkind.New(errkind.KindInvalidSchema, "orm: destination must point to a struct or slice")
	}
}

func nilOrEmptySlice(v reflect.Value) {
	if v.Kind() == reflect.Slice {
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	}
}

func zeroRelationField(field reflect.Value) {
	if field.CanSet() {
		field.Set(reflect.Zero(field.Type()))
	}
}

func isSQLDB(v any) bool {
	_, ok := v.(*sql.DB)
	return ok
}
