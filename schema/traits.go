package schema

import (
	"reflect"
	"strings"
	"sync"
)

type typeMetadata struct {
	columns    []*Column
	fieldPaths map[string][]int
}

var traitMetadataCache sync.Map // map[reflect.Type]*typeMetadata

func ColumnsFromType(t reflect.Type) []*Column {
	meta := discoverTypeMetadata(t)
	if meta == nil {
		return nil
	}
	return cloneColumns(meta.columns)
}

func StructFieldIndexMap(t reflect.Type) map[string][]int {
	meta := discoverTypeMetadata(t)
	if meta == nil {
		return nil
	}
	return cloneFieldPaths(meta.fieldPaths)
}

func discoverTypeMetadata(t reflect.Type) *typeMetadata {
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	if cached, ok := traitMetadataCache.Load(t); ok {
		if meta, ok := cached.(*typeMetadata); ok {
			return meta
		}
	}
	meta := &typeMetadata{
		fieldPaths: map[string][]int{},
	}
	seen := map[reflect.Type]bool{}
	collectTypeMetadata(t, nil, nil, seen, meta)
	traitMetadataCache.Store(t, meta)
	return meta
}

func collectTypeMetadata(t reflect.Type, traitStack []string, indexPath []int, seen map[reflect.Type]bool, meta *typeMetadata) {
	if t == nil || meta == nil {
		return
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	if seen[t] {
		return
	}
	seen[t] = true
	defer delete(seen, t)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		nextPath := append(append([]int(nil), indexPath...), i)
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if field.Anonymous && fieldType.Kind() == reflect.Struct && !isPrimitiveEmbeddedType(fieldType) {
			collectTypeMetadata(fieldType, append(traitStack, fieldType.Name()), nextPath, seen, meta)
			continue
		}
		col := columnFromReflectField(field, traitStack)
		if col == nil {
			continue
		}
		meta.columns = append(meta.columns, col)
		key := strings.ToLower(ToSnakeCase(col.GoName))
		meta.fieldPaths[key] = append([]int(nil), nextPath...)
	}
}

func reflectTypeToSchemaType(t reflect.Type) Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if typ, ok := ResolveCustomType(t); ok {
		return typ
	}
	switch t.Kind() {
	case reflect.Bool:
		return Type{Name: "boolean", Kind: TypeBool}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Type{Name: "bigint", Kind: TypeInt}
	case reflect.Float32, reflect.Float64:
		return Type{Name: "double precision", Kind: TypeFloat}
	case reflect.String:
		return Type{Name: "text", Kind: TypeString}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return Type{Name: "bytea", Kind: TypeBytes}
		}
		elem := reflectTypeToSchemaType(t.Elem())
		return Type{Name: elem.Name + "[]", Kind: TypeArray, ArrayOf: &elem}
	case reflect.Struct:
		if t.PkgPath() == "time" && t.Name() == "Time" {
			return Type{Name: "timestamptz", Kind: TypeTime}
		}
		if strings.EqualFold(t.Name(), "UUID") {
			return Type{Name: "uuid", Kind: TypeUUID}
		}
		return Type{Name: ToSnakeCase(t.Name()), Kind: TypeCustom}
	default:
		return Type{Name: "text", Kind: TypeUnknown}
	}
}

func columnFromReflectField(field reflect.StructField, traitStack []string) *Column {
	tag := field.Tag.Get("orm")
	tagMap := parseORMTag(tag)
	col := &Column{
		Name:   ToSnakeCase(field.Name),
		GoName: field.Name,
		Type:   reflectTypeToSchemaType(field.Type),
		Tags:   map[string]string{},
	}
	if col.Name == "" {
		return nil
	}
	if col.Name == "id" && !tagMap["type"].IsSet && !tagMap["pk"].IsSet {
		col.PrimaryKey = true
		col.Unique = true
	}
	if tagMap["pk"].IsSet {
		col.PrimaryKey = true
		col.Unique = true
	}
	if tagMap["unique"].IsSet {
		col.Unique = true
	}
	if tagMap["identity"].IsSet {
		col.Identity = true
	}
	if tagMap["soft_delete"].IsSet {
		col.SoftDelete = true
		col.Nullable = true
	}
	if tagMap["created_at"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(field.Name, "CreatedAt") {
		col.CreatedAt = true
	}
	if tagMap["updated_at"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(field.Name, "UpdatedAt") {
		col.UpdatedAt = true
	}
	if tagMap["deleted_at"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(field.Name, "DeletedAt") {
		col.DeletedAt = true
		col.Nullable = true
	}
	if tagMap["created_by"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(field.Name, "CreatedBy") {
		col.CreatedBy = true
	}
	if tagMap["updated_by"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(field.Name, "UpdatedBy") {
		col.UpdatedBy = true
	}
	if tagMap["deleted_by"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(field.Name, "DeletedBy") {
		col.DeletedBy = true
	}
	if tagMap["company"].IsSet || hasTrait(traitStack, "Company") && strings.EqualFold(field.Name, "CompanyID") {
		col.Scope = ScopeCompany
	}
	if tagMap["tenant"].IsSet {
		col.Scope = ScopeTenant
	}
	if tagMap["organization"].IsSet {
		col.Scope = ScopeOrganization
	}
	if tagMap["workspace"].IsSet {
		col.Scope = ScopeWorkspace
	}
	if tagMap["warehouse"].IsSet {
		col.Scope = ScopeWarehouse
	}
	if tagMap["user"].IsSet {
		col.Scope = ScopeUser
	}
	if tagMap["default"].IsSet {
		col.Default = tagMap["default"].Value
	}
	if tagMap["generated"].IsSet {
		col.Generated = tagMap["generated"].Value
	}
	if tagMap["nullable"].IsSet {
		col.Nullable = true
	}
	if tagMap["notnull"].IsSet {
		col.Nullable = false
	}
	if col.CreatedAt || col.UpdatedAt || col.DeletedAt {
		col.Type.Kind = TypeTime
		col.Nullable = col.DeletedAt
	}
	if col.CreatedBy || col.UpdatedBy || col.DeletedBy || col.Scope != ScopeNone {
		col.Nullable = false
	}
	if tagMap["type"].IsSet {
		col.Type.Name = tagMap["type"].Value
		col.Type.Kind = typeKindFromName(tagMap["type"].Value)
	}
	if tagMap["array"].IsSet {
		elem := col.Type
		col.Type = Type{Name: col.Type.Name + "[]", Kind: TypeArray, ArrayOf: &elem}
	}
	if tagMap["enum"].IsSet {
		col.Type.Kind = TypeEnum
		col.Type.Name = tagMap["enum"].Value
		if tagMap["values"].IsSet {
			col.Type.EnumValues = splitCSV(tagMap["values"].Value)
		}
	}
	if tagMap["json"].IsSet {
		col.Type.Kind = TypeJSON
		if col.Type.Name == "" {
			col.Type.Name = "jsonb"
		}
	}
	if tagMap["pk"].IsSet && tagMap["nullable"].IsSet {
		col.Nullable = false
	}
	col.Tags = flattenTagMap(tagMap)
	if col.Type.Kind == TypeUnknown {
		col.Type = reflectTypeToSchemaType(field.Type)
	}
	return col
}

func hasTrait(traits []string, name string) bool {
	for _, trait := range traits {
		if strings.EqualFold(trait, name) {
			return true
		}
	}
	return false
}

func flattenTagMap(meta map[string]tagEntry) map[string]string {
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		if !v.IsSet {
			continue
		}
		out[k] = v.Value
	}
	return out
}

func splitCSV(in string) []string {
	in = strings.TrimSpace(in)
	if in == "" {
		return nil
	}
	parts := strings.Split(in, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func isPrimitiveEmbeddedType(t reflect.Type) bool {
	switch t.PkgPath() {
	case "time":
		if t.Name() == "Time" {
			return true
		}
	}
	return false
}

func cloneFieldPaths(src map[string][]int) map[string][]int {
	out := make(map[string][]int, len(src))
	for k, v := range src {
		out[k] = append([]int(nil), v...)
	}
	return out
}
