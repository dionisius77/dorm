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

	"github.com/dionisius77/dorm/errkind"
)

type Builder struct {
	Root string
}

// NewBuilder returns a schema builder rooted at a source tree.
func NewBuilder(root string) *Builder {
	return &Builder{Root: root}
}

// Build parses source files and returns the discovered schema.
func (b *Builder) Build(ctx context.Context) (*Schema, error) {
	var result *Schema
	err := traceOperation(ctx, "db.schema.build", func(ctx context.Context) error {
		if b == nil {
			return errkind.New(errkind.KindConfiguration, "schema: nil builder")
		}
		if b.Root == "" {
			return errkind.New(errkind.KindConfiguration, "schema: empty root")
		}
		fingerprint, err := sourceFingerprint(b.Root)
		if err != nil {
			return err
		}
		if cached, ok := loadCachedSchema(b.Root, fingerprint); ok {
			result = cached
			return nil
		}

		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, b.Root, func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, parser.ParseComments)
		if err != nil {
			return err
		}

		resolver, err := newStructTypeResolver(b.Root)
		if err != nil {
			return err
		}

		out := &Schema{Name: filepath.Base(b.Root), Version: 1}
		for _, pkg := range pkgs {
			structTypes := collectPackageStructTypes(pkg)
			embeddedTypes := collectPackageEmbeddedTypeNames(pkg)
			for _, file := range pkg.Files {
				packageName := file.Name.Name
				imports := collectFileImportMap(file)
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
						doc := commentText(gen.Doc, typeSpec.Doc, file.Doc)
						if viewSQL := directiveValue(doc, "view"); viewSQL != "" {
							out.Views = append(out.Views, buildViewFromType(typeSpec, packageName, doc, viewSQL))
							continue
						}
						if _, ok := embeddedTypes[typeSpec.Name.Name]; ok && directiveValue(doc, "table") == "" {
							continue
						}
						model := buildTableFromType(typeSpec, st, packageName, doc, structTypes, imports, resolver)
						if model != nil {
							out.Tables = append(out.Tables, model)
						}
					}
				}
			}
		}
		out.Sort()
		if err := out.Validate(); err != nil {
			return err
		}
		storeCachedSchema(b.Root, fingerprint, out)
		result = out.Clone()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func buildTableFromType(typeSpec *ast.TypeSpec, st *ast.StructType, packageName, doc string, structTypes map[string]*ast.StructType, imports map[string]string, resolver *structTypeResolver) *Table {
	table := &Table{
		Name:        Pluralize(ToSnakeCase(typeSpec.Name.Name)),
		GoTypeName:  typeSpec.Name.Name,
		PackageName: packageName,
		Metadata:    map[string]string{},
	}
	if doc != "" {
		if name := directiveValue(doc, "table"); name != "" {
			table.Name = name
		}
		if title := directiveValue(doc, "name"); title != "" {
			table.Metadata["name"] = title
		}
	}
	table.Columns = append(table.Columns, buildColumnsFromStruct(typeSpec.Name.Name, st, structTypes, imports, resolver, table, nil, map[string]bool{})...)
	if !hasPrimaryKey(table) {
		if c := columnByName(table.Columns, "id"); c != nil {
			c.PrimaryKey = true
			c.Unique = true
		}
	}
	if pk := primaryKeyColumns(table.Columns); len(pk) > 0 {
		table.Constraints = append(table.Constraints, &Constraint{
			Name:    table.Name + "_pkey",
			Kind:    ConstraintPrimaryKey,
			Columns: pk,
		})
	}
	for _, col := range table.Columns {
		if col == nil || !col.Unique || col.PrimaryKey {
			continue
		}
		name := table.Name + "_" + col.Name + "_key"
		if constraintExists(table.Constraints, name) {
			continue
		}
		table.Constraints = append(table.Constraints, &Constraint{
			Name:    name,
			Kind:    ConstraintUnique,
			Columns: []string{col.Name},
		})
	}
	return table
}

func buildViewFromType(typeSpec *ast.TypeSpec, packageName, doc, sqlText string) *View {
	view := &View{
		Name: ToSnakeCase(typeSpec.Name.Name),
		SQL:  sqlText,
		Metadata: map[string]string{
			"type":    typeSpec.Name.Name,
			"package": packageName,
		},
	}
	if doc != "" {
		if name := directiveValue(doc, "name"); name != "" {
			view.Name = name
			view.Metadata["name"] = name
		} else if tableName := directiveValue(doc, "table"); tableName != "" {
			view.Name = tableName
			view.Metadata["name"] = tableName
		}
		if materialized := directiveValue(doc, "materialized"); strings.EqualFold(materialized, "true") {
			view.Materialized = true
		}
	}
	return view
}

func buildColumnsFromStruct(typeName string, st *ast.StructType, structTypes map[string]*ast.StructType, imports map[string]string, resolver *structTypeResolver, table *Table, traitStack []string, seen map[string]bool) []*Column {
	if st == nil {
		return nil
	}
	if seen == nil {
		seen = map[string]bool{}
	}
	if seen[typeName] {
		return nil
	}
	seen[typeName] = true
	defer delete(seen, typeName)

	var cols []*Column
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			embedded, ok := parseEmbeddedTypeRef(field.Type)
			if !ok {
				continue
			}
			if nested, nestedStructTypes, ok := resolveEmbeddedStruct(embedded, structTypes, imports, resolver); ok {
				cols = append(cols, buildColumnsFromStruct(embedded.Name, nested, nestedStructTypes, imports, resolver, table, append(traitStack, embedded.Name), seen)...)
			}
			continue
		}
		for _, name := range field.Names {
			col := buildColumn(name.Name, field, table, traitStack)
			if col == nil {
				continue
			}
			cols = append(cols, col)
		}
	}
	return cols
}

func buildColumn(goName string, field *ast.Field, table *Table, traitStack []string) *Column {
	tag := ""
	if field.Tag != nil {
		if unquoted, err := strconv.Unquote(field.Tag.Value); err == nil {
			tag = unquoted
		}
	}
	tagMap := parseStructTag(tag)
	meta := parseORMTag(tagMap["orm"])

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
	if meta["created_at"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(goName, "CreatedAt") {
		col.CreatedAt = true
	}
	if meta["updated_at"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(goName, "UpdatedAt") {
		col.UpdatedAt = true
	}
	if meta["deleted_at"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(goName, "DeletedAt") {
		col.DeletedAt = true
		col.Nullable = true
	}
	if meta["created_by"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(goName, "CreatedBy") {
		col.CreatedBy = true
	}
	if meta["updated_by"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(goName, "UpdatedBy") {
		col.UpdatedBy = true
	}
	if meta["deleted_by"].IsSet || hasTrait(traitStack, "Audit") && strings.EqualFold(goName, "DeletedBy") {
		col.DeletedBy = true
	}
	if meta["version"].IsSet || hasTrait(traitStack, "Version") && strings.EqualFold(goName, "Version") {
		col.Version = true
		col.Type = Type{Name: "bigint", Kind: TypeInt}
		col.Nullable = false
		col.Default = "1"
	}
	if meta["company"].IsSet || hasTrait(traitStack, "Company") && strings.EqualFold(goName, "CompanyID") {
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
		if values := meta["values"].Value; values != "" {
			col.Type.EnumValues = strings.Split(values, "|")
		}
	}
	if meta["index"].IsSet && table != nil {
		table.Indexes = append(table.Indexes, &Index{
			Name:    "idx_" + table.Name + "_" + col.Name,
			Columns: []string{col.Name},
		})
	}
	if meta["where"].IsSet && table != nil {
		table.Indexes = append(table.Indexes, &Index{
			Name:    "idx_" + table.Name + "_" + col.Name,
			Columns: []string{col.Name},
			Where:   meta["where"].Value,
		})
	}
	if meta["json"].IsSet {
		col.Type.Kind = TypeJSON
		col.Type.Name = "jsonb"
	}
	if col.Version {
		col.Type = Type{Name: "bigint", Kind: TypeInt}
		col.Nullable = false
		col.Default = "1"
	}
	if meta["array"].IsSet {
		col.Type.Kind = TypeArray
	}
	col.Tags = tagMap
	return col
}

func collectPackageStructTypes(pkg *ast.Package) map[string]*ast.StructType {
	out := map[string]*ast.StructType{}
	if pkg == nil {
		return out
	}
	for _, file := range pkg.Files {
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
				if !ok {
					continue
				}
				out[typeSpec.Name.Name] = st
			}
		}
	}
	return out
}

func collectPackageEmbeddedTypeNames(pkg *ast.Package) map[string]struct{} {
	out := map[string]struct{}{}
	if pkg == nil {
		return out
	}
	for _, file := range pkg.Files {
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
				if !ok {
					continue
				}
				for _, field := range st.Fields.List {
					if len(field.Names) != 0 {
						continue
					}
					if name := embeddedTypeName(field.Type); name != "" {
						out[name] = struct{}{}
					}
				}
			}
		}
	}
	return out
}

func embeddedTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return embeddedTypeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
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
		if typ, ok := ResolveCustomTypeName(t.Name); ok {
			return typ
		}
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
		if typ, ok := ResolveCustomTypeName(full); ok {
			return typ
		}
		if typ, ok := ResolveCustomTypeName(name); ok {
			return typ
		}
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

// ResolveCustomTypeName looks up a registered custom schema type by name.
func ResolveCustomTypeName(name string) (Type, bool) {
	name = normalizeCustomTypeName(name)
	if name == "" {
		return Type{}, false
	}
	customTypeMu.RLock()
	defer customTypeMu.RUnlock()
	if typ, ok := customTypeByName[name]; ok {
		return typ, true
	}
	return Type{}, false
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

func constraintExists(constraints []*Constraint, name string) bool {
	for _, c := range constraints {
		if c != nil && c.Name == name {
			return true
		}
	}
	return false
}
