package orm

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/dionisius77/dorm/schema"
)

type BeforeCreateHook interface {
	BeforeCreate(context.Context) error
}

type AfterCreateHook interface {
	AfterCreate(context.Context) error
}

type BeforeUpdateHook interface {
	BeforeUpdate(context.Context) error
}

type AfterUpdateHook interface {
	AfterUpdate(context.Context) error
}

type BeforeDeleteHook interface {
	BeforeDelete(context.Context) error
}

type AfterDeleteHook interface {
	AfterDelete(context.Context) error
}

type BeforeFindHook interface {
	BeforeFind(context.Context) error
}

type AfterFindHook interface {
	AfterFind(context.Context) error
}

func (s *Session) Load(dest any, relation string) error {
	if s == nil {
		return fmt.Errorf("orm: nil session")
	}
	if relation == "" {
		return fmt.Errorf("orm: empty relation name")
	}
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("orm: load destination must be a non-nil pointer")
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
		return fmt.Errorf("orm: invalid relation destination")
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
				return fmt.Errorf("orm: preload destination must contain structs")
			}
			if err := s.loadStructRelation(item, relation); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("orm: preload destination must be a struct or slice")
	}
}

func (s *Session) loadStructRelation(parent reflect.Value, relation string) error {
	field, fieldInfo, ok := structFieldByName(parent, relation)
	if !ok {
		return fmt.Errorf("orm: relation %q not found", relation)
	}

	switch field.Kind() {
	case reflect.Struct, reflect.Pointer:
		return s.loadBelongsToRelation(parent, field, fieldInfo, relation)
	case reflect.Slice:
		return s.loadHasManyRelation(parent, field, fieldInfo, relation)
	default:
		return fmt.Errorf("orm: relation %q is not loadable", relation)
	}
}

func (s *Session) loadBelongsToRelation(parent, relationField reflect.Value, relationType reflect.StructField, relation string) error {
	targetType := relationField.Type()
	if targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	if targetType.Kind() != reflect.Struct {
		return fmt.Errorf("orm: relation %q is not a struct or pointer to struct", relation)
	}

	fkField, ok := structFieldByCandidates(parent, relation+"ID", relation+"Id", schema.ToSnakeCase(relation)+"_id")
	if !ok {
		return fmt.Errorf("orm: parent model missing foreign key for relation %q", relation)
	}
	if isZeroValue(fkField.Interface()) {
		relationField.Set(reflect.Zero(relationField.Type()))
		return nil
	}

	targetTable := s.db.lookupTableForType(targetType)
	pkCols := primaryKeyColumnsForTable(targetTable)
	if len(pkCols) != 1 {
		return fmt.Errorf("orm: relation %q requires a single-column primary key", relation)
	}

	sqlText, err := s.db.dialect.RenderSelect(targetTable.Name, quoteColumns(columnsForTable(targetTable), s.db.dialect), []string{quoteIdent(pkCols[0]) + " = " + placeholder(1)}, nil, nil, nil)
	if err != nil {
		return err
	}
	targetSlice := reflect.New(reflect.SliceOf(targetType))
	rows, err := s.db.queryContext(s.ctx, sqlText, fkField.Interface())
	if err != nil {
		return err
	}
	defer rows.Close()
	if err := scanIntoSlice(rows, targetSlice.Interface(), targetTable); err != nil {
		return err
	}
	if err := invokeAfterFindHooks(s.ctx, targetSlice.Elem()); err != nil {
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
		return fmt.Errorf("orm: relation %q is not a slice of structs", relation)
	}

	parentTable := s.db.lookupTableForType(parent.Type())
	parentPKField, ok := primaryKeyField(parent, parentTable)
	if !ok {
		return fmt.Errorf("orm: parent model missing primary key for relation %q", relation)
	}
	if isZeroValue(parentPKField.Interface()) {
		relationField.Set(reflect.MakeSlice(relationField.Type(), 0, 0))
		return nil
	}

	childTable := s.db.lookupTableForType(elemType)
	fkName := schema.ToSnakeCase(parentTable.GoTypeName) + "_id"
	where := []string{quoteIdent(fkName) + " = " + placeholder(1)}
	sqlText, err := s.db.dialect.RenderSelect(childTable.Name, quoteColumns(columnsForTable(childTable), s.db.dialect), where, nil, nil, nil)
	if err != nil {
		return err
	}
	targetSlice := reflect.New(relationField.Type())
	rows, err := s.db.queryContext(s.ctx, sqlText, parentPKField.Interface())
	if err != nil {
		return err
	}
	defer rows.Close()
	if err := scanIntoSlice(rows, targetSlice.Interface(), childTable); err != nil {
		return err
	}
	if err := invokeAfterFindHooks(s.ctx, targetSlice.Elem()); err != nil {
		return err
	}
	relationField.Set(targetSlice.Elem())
	return nil
}

func invokeBeforeCreateHook(ctx context.Context, model any) error {
	if hook, ok := model.(BeforeCreateHook); ok {
		return hook.BeforeCreate(ctx)
	}
	return nil
}

func invokeAfterCreateHook(ctx context.Context, model any) error {
	if hook, ok := model.(AfterCreateHook); ok {
		return hook.AfterCreate(ctx)
	}
	return nil
}

func invokeBeforeUpdateHook(ctx context.Context, model any) error {
	if hook, ok := model.(BeforeUpdateHook); ok {
		return hook.BeforeUpdate(ctx)
	}
	return nil
}

func invokeAfterUpdateHook(ctx context.Context, model any) error {
	if hook, ok := model.(AfterUpdateHook); ok {
		return hook.AfterUpdate(ctx)
	}
	return nil
}

func invokeBeforeDeleteHook(ctx context.Context, model any) error {
	if hook, ok := model.(BeforeDeleteHook); ok {
		return hook.BeforeDelete(ctx)
	}
	return nil
}

func invokeAfterDeleteHook(ctx context.Context, model any) error {
	if hook, ok := model.(AfterDeleteHook); ok {
		return hook.AfterDelete(ctx)
	}
	return nil
}

func invokeBeforeFindHook(ctx context.Context, dest any) error {
	if hook, ok := dest.(BeforeFindHook); ok {
		return hook.BeforeFind(ctx)
	}
	return nil
}

func invokeAfterFindHooks(ctx context.Context, rv reflect.Value) error {
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Slice:
		for i := 0; i < rv.Len(); i++ {
			if err := invokeAfterFindHooks(ctx, rv.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return invokeAfterFindHooks(ctx, rv.Elem())
	case reflect.Struct:
		if rv.CanAddr() {
			if hook, ok := rv.Addr().Interface().(AfterFindHook); ok {
				return hook.AfterFind(ctx)
			}
		}
		if hook, ok := rv.Interface().(AfterFindHook); ok {
			return hook.AfterFind(ctx)
		}
		return nil
	default:
		return nil
	}
}

func structFieldByName(rv reflect.Value, name string) (reflect.Value, reflect.StructField, bool) {
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if strings.EqualFold(field.Name, name) || strings.EqualFold(schema.ToSnakeCase(field.Name), schema.ToSnakeCase(name)) {
			return rv.Field(i), field, true
		}
	}
	return reflect.Value{}, reflect.StructField{}, false
}

func structFieldByCandidates(rv reflect.Value, candidates ...string) (reflect.Value, bool) {
	for _, candidate := range candidates {
		if field, _, ok := structFieldByName(rv, candidate); ok {
			return field, true
		}
	}
	return reflect.Value{}, false
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
	sqlText, err := s.db.dialect.RenderSelect(table.Name, quoteColumns(columnsForTable(table), s.db.dialect), where, nil, nil, nil)
	if err != nil {
		return err
	}
	rows, err := s.db.queryContext(s.ctx, sqlText, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanIntoSlice(rows, target, table)
}

func ensurePointerToStructOrSlice(v reflect.Value) error {
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("orm: destination must be a non-nil pointer")
	}
	switch v.Elem().Kind() {
	case reflect.Struct, reflect.Slice:
		return nil
	default:
		return fmt.Errorf("orm: destination must point to a struct or slice")
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
