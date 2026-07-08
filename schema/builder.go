package schema

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Builder struct {
	Root string
}

func NewBuilder(root string) *Builder {
	return &Builder{Root: root}
}

func (b *Builder) Build(ctx context.Context) (*Schema, error) {
	_ = ctx
	if b == nil {
		return nil, fmt.Errorf("schema: nil builder")
	}
	if b.Root == "" {
		return nil, fmt.Errorf("schema: empty root")
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, b.Root, func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	out := &Schema{Name: filepath.Base(b.Root), Version: 1}
	for pkgName, pkg := range pkgs {
		_ = pkgName
		for fileName, file := range pkg.Files {
			_ = fileName
			packageName := file.Name.Name
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					st, ok := typeSpec.Type.(*ast.StructType)
					if !ok || !ast.IsExported(typeSpec.Name.Name) {
						continue
					}
					model := buildTableFromType(typeSpec, st, packageName, file)
					if model != nil {
						out.Tables = append(out.Tables, model)
					}
				}
			}
		}
	}
	out.Sort()
	if err := out.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}

func buildTableFromType(typeSpec *ast.TypeSpec, st *ast.StructType, packageName string, file *ast.File) *Table {
	table := &Table{
		Name:        Pluralize(ToSnakeCase(typeSpec.Name.Name)),
		GoTypeName:  typeSpec.Name.Name,
		PackageName: packageName,
		Metadata:    map[string]string{},
	}
	if doc := commentText(typeSpec.Doc, file.Doc); doc != "" {
		if name := directiveValue(doc, "table"); name != "" {
			table.Name = name
		}
		if title := directiveValue(doc, "name"); title != "" {
			table.Metadata["name"] = title
		}
	}
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		for _, name := range field.Names {
			col := buildColumn(name.Name, field, table)
			if col != nil {
				table.Columns = append(table.Columns, col)
			}
		}
	}
	if !hasPrimaryKey(table) {
		if c := columnByName(table.Columns, "id"); c != nil {
			c.PrimaryKey = true
			c.Unique = true
		}
	}
	if pk := primaryKeyColumns(table.Columns); len(pk) > 0 {
		table.Constraints = append(table.Constraints, &Constraint{
			Name:    "pk_" + table.Name,
			Kind:    ConstraintPrimaryKey,
			Columns: pk,
		})
	}
	return table
}

func buildColumn(goName string, field *ast.Field, table *Table) *Column {
	tag := ""
	if field.Tag != nil {
		if unquoted, err := strconv.Unquote(field.Tag.Value); err == nil {
			tag = unquoted
		}
	}
	tagMap := parseStructTag(tag)
	ormTag := tagMap["orm"]
	meta := parseORMTag(ormTag)

	col := &Column{
		Name:   ToSnakeCase(goName),
		GoName: goName,
		Type:   inferType(field.Type, meta),
		Tags:   map[string]string{},
	}
	if col.Name == "" {
		return nil
	}
	if col.Name == "id" && !meta["type"].IsSet && !meta["pk"].IsSet {
		col.PrimaryKey = true
		col.Unique = true
	}
	if meta["pk"].IsSet {
		col.PrimaryKey = true
		col.Unique = true
	}
	if meta["unique"].IsSet {
		col.Unique = true
	}
	if meta["identity"].IsSet {
		col.Identity = true
	}
	if meta["soft_delete"].IsSet {
		col.SoftDelete = true
		col.Nullable = true
	}
	if meta["created_at"].IsSet {
		col.CreatedAt = true
	}
	if meta["updated_at"].IsSet {
		col.UpdatedAt = true
	}
	if meta["deleted_at"].IsSet {
		col.DeletedAt = true
		col.Nullable = true
	}
	if meta["created_by"].IsSet {
		col.CreatedBy = true
	}
	if meta["updated_by"].IsSet {
		col.UpdatedBy = true
	}
	if meta["deleted_by"].IsSet {
		col.DeletedBy = true
	}
	if meta["company"].IsSet {
		col.Scope = ScopeCompany
	}
	if meta["tenant"].IsSet {
		col.Scope = ScopeTenant
	}
	if meta["organization"].IsSet {
		col.Scope = ScopeOrganization
	}
	if meta["workspace"].IsSet {
		col.Scope = ScopeWorkspace
	}
	if meta["warehouse"].IsSet {
		col.Scope = ScopeWarehouse
	}
	if meta["user"].IsSet {
		col.Scope = ScopeUser
	}
	if meta["default"].IsSet {
		col.Default = meta["default"].Value
	}
	if meta["generated"].IsSet {
		col.Generated = meta["generated"].Value
	}
	if meta["nullable"].IsSet {
		col.Nullable = true
	}
	if meta["notnull"].IsSet {
		col.Nullable = false
	}
	if col.CreatedAt || col.UpdatedAt || col.DeletedAt {
		col.Type.Kind = TypeTime
		col.Nullable = col.DeletedAt
	}
	if col.CreatedBy || col.UpdatedBy || col.DeletedBy || col.Scope != ScopeNone {
		col.Nullable = false
	}
	if meta["type"].IsSet {
		col.Type.Name = meta["type"].Value
		col.Type.Kind = typeKindFromName(meta["type"].Value)
	}
	if meta["array"].IsSet {
		elem := col.Type
		col.Type = Type{Name: col.Type.Name + "[]", Kind: TypeArray, ArrayOf: &elem}
	}
	if meta["enum"].IsSet {
		col.Type.Kind = TypeEnum
		col.Type.Name = meta["enum"].Value
		values := meta["values"].Value
		if values != "" {
			col.Type.EnumValues = strings.Split(values, "|")
		}
	}
	if meta["index"].IsSet {
		table.Indexes = append(table.Indexes, &Index{
			Name:    "idx_" + table.Name + "_" + col.Name,
			Columns: []string{col.Name},
		})
	}
	if meta["where"].IsSet {
		table.Indexes = append(table.Indexes, &Index{
			Name:    "idx_" + table.Name + "_" + col.Name,
			Columns: []string{col.Name},
			Where:   meta["where"].Value,
		})
	}
	if tagMap["json"] == "true" {
		col.Type.Kind = TypeJSON
		col.Type.Name = "jsonb"
	}
	if tagMap["array"] == "true" {
		col.Type.Kind = TypeArray
	}
	if tagMap["pk"] == "true" && !col.PrimaryKey {
		col.PrimaryKey = true
		col.Unique = true
	}
	if tagMap["unique"] == "true" && !col.Unique {
		col.Unique = true
	}
	col.Tags = tagMap
	return col
}

type tagEntry struct {
	Value string
	IsSet bool
}

func parseORMTag(in string) map[string]tagEntry {
	out := map[string]tagEntry{}
	if in == "" {
		return out
	}
	parts := strings.Split(in, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "=") {
			items := strings.SplitN(part, "=", 2)
			out[strings.TrimSpace(items[0])] = tagEntry{Value: strings.TrimSpace(items[1]), IsSet: true}
			continue
		}
		out[part] = tagEntry{Value: "true", IsSet: true}
	}
	return out
}

func parseStructTag(in string) map[string]string {
	out := map[string]string{}
	if in == "" {
		return out
	}
	parts := strings.Split(in, " ")
	for _, part := range parts {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := kv[0]
		val, err := strconv.Unquote(strings.TrimSpace(kv[1]))
		if err != nil {
			val = strings.Trim(strings.TrimSpace(kv[1]), "\"")
		}
		out[key] = val
	}
	return out
}

func commentText(groups ...*ast.CommentGroup) string {
	var parts []string
	for _, g := range groups {
		if g == nil {
			continue
		}
		for _, c := range g.List {
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

var directiveRe = regexp.MustCompile(`(?m)^orm:([a-zA-Z0-9_]+)=(.+)$`)

func directiveValue(text, key string) string {
	matches := directiveRe.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		if match[1] == key {
			return strings.TrimSpace(match[2])
		}
	}
	return ""
}

func typeKindFromName(name string) TypeKind {
	switch strings.ToLower(name) {
	case "bool", "boolean":
		return TypeBool
	case "int", "int2", "int4", "int8", "smallint", "integer", "bigint", "serial", "bigserial":
		return TypeInt
	case "float", "float4", "float8", "double", "numeric", "decimal":
		return TypeFloat
	case "text", "string", "varchar", "char", "character varying":
		return TypeString
	case "bytea", "bytes", "blob":
		return TypeBytes
	case "timestamp", "timestamptz", "time", "date", "datetime":
		return TypeTime
	case "uuid":
		return TypeUUID
	case "json", "jsonb":
		return TypeJSON
	case "array":
		return TypeArray
	default:
		return TypeCustom
	}
}

func inferType(expr ast.Expr, meta map[string]tagEntry) Type {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string":
			return Type{Name: "text", Kind: TypeString}
		case "bool":
			return Type{Name: "boolean", Kind: TypeBool}
		case "byte":
			return Type{Name: "smallint", Kind: TypeInt}
		case "int", "int32", "int64", "uint", "uint32", "uint64":
			return Type{Name: "bigint", Kind: TypeInt}
		case "float32", "float64":
			return Type{Name: "double precision", Kind: TypeFloat}
		case "Time":
			return Type{Name: "timestamptz", Kind: TypeTime}
		case "UUID":
			return Type{Name: "uuid", Kind: TypeUUID}
		default:
			if strings.EqualFold(t.Name, "time") {
				return Type{Name: "timestamptz", Kind: TypeTime}
			}
			return Type{Name: ToSnakeCase(t.Name), Kind: TypeCustom}
		}
	case *ast.SelectorExpr:
		pkg := exprString(t.X)
		name := t.Sel.Name
		full := pkg + "." + name
		switch full {
		case "time.Time":
			return Type{Name: "timestamptz", Kind: TypeTime}
		case "uuid.UUID":
			return Type{Name: "uuid", Kind: TypeUUID}
		case "json.RawMessage":
			return Type{Name: "jsonb", Kind: TypeJSON}
		default:
			return Type{Name: ToSnakeCase(name), Kind: TypeCustom}
		}
	case *ast.ArrayType:
		elem := inferType(t.Elt, meta)
		return Type{Name: elem.Name + "[]", Kind: TypeArray, ArrayOf: &elem}
	case *ast.StarExpr:
		typ := inferType(t.X, meta)
		typ.Nullable = true
		return typ
	default:
		return Type{Name: "text", Kind: TypeUnknown}
	}
}

func exprString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.ArrayType:
		return "[]" + exprString(t.Elt)
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func hasPrimaryKey(t *Table) bool {
	for _, c := range t.Columns {
		if c.PrimaryKey {
			return true
		}
	}
	return false
}

func primaryKeyColumns(cols []*Column) []string {
	var out []string
	for _, c := range cols {
		if c.PrimaryKey {
			out = append(out, c.Name)
		}
	}
	sort.Strings(out)
	return out
}

func columnByName(cols []*Column, name string) *Column {
	for _, c := range cols {
		if c.Name == name {
			return c
		}
	}
	return nil
}
