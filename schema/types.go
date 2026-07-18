package schema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dionisius77/dorm/errkind"
)

// Schema is the in-memory representation of discovered database structure.
type Schema struct {
	Name    string
	Version int
	Tables  []*Table
	Enums   []*EnumType
	Views   []*View
}

// Table describes a database table or view backing structure.
type Table struct {
	Name        string
	GoTypeName  string
	PackageName string
	Columns     []*Column
	Indexes     []*Index
	Constraints []*Constraint
	Comments    []string
	Metadata    map[string]string
}

// Column describes a database column and its inferred metadata.
type Column struct {
	Name          string
	GoName        string
	Type          Type
	Nullable      bool
	PrimaryKey    bool
	Unique        bool
	Identity      bool
	AutoIncrement bool
	Default       string
	Generated     string
	SoftDelete    bool
	CreatedAt     bool
	UpdatedAt     bool
	DeletedAt     bool
	CreatedBy     bool
	UpdatedBy     bool
	DeletedBy     bool
	Version       bool
	Scope         ScopeKind
	Tags          map[string]string
}

// Index describes a database index definition.
type Index struct {
	Name       string
	Columns    []string
	Unique     bool
	Expression string
	Where      string
	Method     string
	Metadata   map[string]string
}

// ConstraintKind identifies a constraint category.
type ConstraintKind string

const (
	ConstraintPrimaryKey ConstraintKind = "primary_key"
	ConstraintUnique     ConstraintKind = "unique"
	ConstraintForeignKey ConstraintKind = "foreign_key"
	ConstraintCheck      ConstraintKind = "check"
)

// Constraint describes a table constraint.
type Constraint struct {
	Name              string
	Kind              ConstraintKind
	Columns           []string
	ReferencedTable   string
	ReferencedColumns []string
	OnDelete          string
	OnUpdate          string
	Expression        string
	Metadata          map[string]string
}

// TypeKind identifies the broad kind of a column type.
type TypeKind string

const (
	TypeUnknown TypeKind = "unknown"
	TypeBool    TypeKind = "bool"
	TypeInt     TypeKind = "int"
	TypeFloat   TypeKind = "float"
	TypeString  TypeKind = "string"
	TypeBytes   TypeKind = "bytes"
	TypeTime    TypeKind = "time"
	TypeUUID    TypeKind = "uuid"
	TypeJSON    TypeKind = "json"
	TypeArray   TypeKind = "array"
	TypeEnum    TypeKind = "enum"
	TypeCustom  TypeKind = "custom"
)

// Type describes a database type with optional metadata.
type Type struct {
	Name       string
	Kind       TypeKind
	Length     int
	Precision  int
	Scale      int
	Nullable   bool
	ArrayOf    *Type
	EnumValues []string
	Metadata   map[string]string
}

// ScopeKind identifies an access-control scope attached to a column.
type ScopeKind string

const (
	ScopeNone         ScopeKind = ""
	ScopeCompany      ScopeKind = "company"
	ScopeTenant       ScopeKind = "tenant"
	ScopeOrganization ScopeKind = "organization"
	ScopeWorkspace    ScopeKind = "workspace"
	ScopeWarehouse    ScopeKind = "warehouse"
	ScopeUser         ScopeKind = "user"
)

// View describes a database view definition.
type View struct {
	Name         string
	SQL          string
	Materialized bool
	Metadata     map[string]string
}

func (s *Schema) Clone() *Schema {
	if s == nil {
		return nil
	}
	out := *s
	out.Tables = cloneTables(s.Tables)
	out.Enums = cloneEnums(s.Enums)
	out.Views = cloneViews(s.Views)
	return &out
}

func cloneTables(src []*Table) []*Table {
	out := make([]*Table, 0, len(src))
	for _, t := range src {
		if t == nil {
			continue
		}
		tt := *t
		tt.Columns = cloneColumns(t.Columns)
		tt.Indexes = cloneIndexes(t.Indexes)
		tt.Constraints = cloneConstraints(t.Constraints)
		tt.Comments = append([]string(nil), t.Comments...)
		if t.Metadata != nil {
			tt.Metadata = cloneStringMap(t.Metadata)
		}
		out = append(out, &tt)
	}
	return out
}

func cloneColumns(src []*Column) []*Column {
	out := make([]*Column, 0, len(src))
	for _, c := range src {
		if c == nil {
			continue
		}
		cc := *c
		if c.Tags != nil {
			cc.Tags = cloneStringMap(c.Tags)
		}
		if c.Type.ArrayOf != nil {
			arr := *c.Type.ArrayOf
			cc.Type.ArrayOf = &arr
		}
		if c.Type.Metadata != nil {
			cc.Type.Metadata = cloneStringMap(c.Type.Metadata)
		}
		out = append(out, &cc)
	}
	return out
}

func cloneIndexes(src []*Index) []*Index {
	out := make([]*Index, 0, len(src))
	for _, i := range src {
		if i == nil {
			continue
		}
		ii := *i
		ii.Columns = append([]string(nil), i.Columns...)
		if i.Metadata != nil {
			ii.Metadata = cloneStringMap(i.Metadata)
		}
		out = append(out, &ii)
	}
	return out
}

func cloneConstraints(src []*Constraint) []*Constraint {
	out := make([]*Constraint, 0, len(src))
	for _, c := range src {
		if c == nil {
			continue
		}
		cc := *c
		cc.Columns = append([]string(nil), c.Columns...)
		cc.ReferencedColumns = append([]string(nil), c.ReferencedColumns...)
		if c.Metadata != nil {
			cc.Metadata = cloneStringMap(c.Metadata)
		}
		out = append(out, &cc)
	}
	return out
}

func cloneEnums(src []*EnumType) []*EnumType {
	out := make([]*EnumType, 0, len(src))
	for _, e := range src {
		if e == nil {
			continue
		}
		ee := *e
		ee.Values = append([]string(nil), e.Values...)
		if e.Metadata != nil {
			ee.Metadata = cloneStringMap(e.Metadata)
		}
		out = append(out, &ee)
	}
	return out
}

func cloneViews(src []*View) []*View {
	out := make([]*View, 0, len(src))
	for _, v := range src {
		if v == nil {
			continue
		}
		vv := *v
		if v.Metadata != nil {
			vv.Metadata = cloneStringMap(v.Metadata)
		}
		out = append(out, &vv)
	}
	return out
}

func cloneStringMap(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

type EnumType struct {
	Name     string
	Values   []string
	Metadata map[string]string
}

func (s *Schema) Sort() {
	if s == nil {
		return
	}
	sort.SliceStable(s.Tables, func(i, j int) bool { return s.Tables[i].Name < s.Tables[j].Name })
	for _, t := range s.Tables {
		sort.SliceStable(t.Columns, func(i, j int) bool { return t.Columns[i].Name < t.Columns[j].Name })
		sort.SliceStable(t.Indexes, func(i, j int) bool { return t.Indexes[i].Name < t.Indexes[j].Name })
		sort.SliceStable(t.Constraints, func(i, j int) bool { return t.Constraints[i].Name < t.Constraints[j].Name })
	}
	sort.SliceStable(s.Enums, func(i, j int) bool { return s.Enums[i].Name < s.Enums[j].Name })
	sort.SliceStable(s.Views, func(i, j int) bool { return s.Views[i].Name < s.Views[j].Name })
}

func (s *Schema) Validate() error {
	if s == nil {
		return errkind.New(errkind.KindInvalidSchema, "schema: nil schema")
	}
	names := make(map[string]struct{}, len(s.Tables))
	for _, t := range s.Tables {
		if t == nil {
			return errkind.New(errkind.KindInvalidSchema, "schema: nil table")
		}
		if t.Name == "" {
			return errkind.New(errkind.KindInvalidSchema, "schema: table missing name")
		}
		if _, ok := names[t.Name]; ok {
			return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("schema: duplicate table %q", t.Name))
		}
		names[t.Name] = struct{}{}
		columnNames := make(map[string]struct{}, len(t.Columns))
		for _, c := range t.Columns {
			if c == nil {
				return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("schema: table %s has nil column", t.Name))
			}
			if c.Name == "" {
				return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("schema: table %s has column with empty name", t.Name))
			}
			if _, ok := columnNames[c.Name]; ok {
				return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("schema: table %s has duplicate column %q", t.Name, c.Name))
			}
			columnNames[c.Name] = struct{}{}
		}
	}
	return nil
}

func (c Column) IsAuditField() bool {
	return c.CreatedAt || c.UpdatedAt || c.DeletedAt || c.CreatedBy || c.UpdatedBy || c.DeletedBy
}

func (c Column) IsScopeField() bool {
	return c.Scope != ScopeNone
}

func (c Column) TagsString() string {
	if len(c.Tags) == 0 {
		return ""
	}
	keys := make([]string, 0, len(c.Tags))
	for k := range c.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(c.Tags[k])
	}
	return b.String()
}
