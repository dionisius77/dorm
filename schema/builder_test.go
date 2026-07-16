package schema_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/schema"
)

func TestBuilderParsesModelMetadata(t *testing.T) {
	dir := t.TempDir()
	src := `package models

import "time"

// orm:table=products
type Product struct {
    ID string ` + "`orm:\"pk\"`" + `
    CompanyID string ` + "`orm:\"company\"`" + `
    CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
    DeletedAt *time.Time ` + "`orm:\"soft_delete\"`" + `
    Name string ` + "`orm:\"unique\"`" + `
}
`
	if err := os.WriteFile(filepath.Join(dir, "product.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := schema.NewBuilder(dir).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(s.Tables))
	}
	table := s.Tables[0]
	if table.Name != "products" {
		t.Fatalf("expected products table, got %s", table.Name)
	}
	if len(table.Columns) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(table.Columns))
	}
	var idColumn *schema.Column
	for _, col := range table.Columns {
		if col.Name == "id" {
			idColumn = col
			break
		}
	}
	if idColumn == nil || !idColumn.PrimaryKey {
		t.Fatalf("expected ID column to be primary key")
	}
	var uniqueConstraint *schema.Constraint
	for _, c := range table.Constraints {
		if c != nil && c.Kind == schema.ConstraintUnique && len(c.Columns) == 1 && c.Columns[0] == "name" {
			uniqueConstraint = c
			break
		}
	}
	if uniqueConstraint == nil {
		t.Fatalf("expected unique constraint for name, got %#v", table.Constraints)
	}
	if uniqueConstraint.Name != "products_name_key" {
		t.Fatalf("expected unique constraint name products_name_key, got %s", uniqueConstraint.Name)
	}
}

func TestBuilderParsesViews(t *testing.T) {
	dir := t.TempDir()
	src := `package models

// orm:name=active_users
// orm:view=SELECT id, email FROM users WHERE deleted_at IS NULL
// orm:materialized=true
type ActiveUsers struct{}
`
	if err := os.WriteFile(filepath.Join(dir, "active_users.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := schema.NewBuilder(dir).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 0 {
		t.Fatalf("expected 0 tables, got %d", len(s.Tables))
	}
	if len(s.Views) != 1 {
		t.Fatalf("expected 1 view, got %d", len(s.Views))
	}
	view := s.Views[0]
	if view.Name != "active_users" {
		t.Fatalf("expected active_users view, got %s", view.Name)
	}
	if !view.Materialized {
		t.Fatalf("expected materialized view")
	}
	if view.SQL != "SELECT id, email FROM users WHERE deleted_at IS NULL" {
		t.Fatalf("unexpected SQL: %s", view.SQL)
	}
	if view.Metadata["type"] != "ActiveUsers" {
		t.Fatalf("expected type metadata, got %v", view.Metadata)
	}
}

func TestBuilderReturnsConfigurationError(t *testing.T) {
	_, err := schema.NewBuilder("").Build(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errkind.ErrConfiguration) {
		t.Fatalf("expected configuration error, got %T %v", err, err)
	}
}

func TestBuilderUsesRegisteredCustomType(t *testing.T) {
	schema.RegisterCustomType("currency", schema.Type{Name: "numeric", Kind: schema.TypeFloat})
	dir := t.TempDir()
	src := `package models

type Money struct {
    Amount Currency
}
`
	if err := os.WriteFile(filepath.Join(dir, "money.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := schema.NewBuilder(dir).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(s.Tables))
	}
	if len(s.Tables[0].Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(s.Tables[0].Columns))
	}
	if got := s.Tables[0].Columns[0].Type.Name; got != "numeric" {
		t.Fatalf("expected registered custom type mapping, got %s", got)
	}
}

func TestBuilderFlattensEmbeddedTraits(t *testing.T) {
	dir := t.TempDir()
	src := `package models

import "time"

type Company struct {
    CompanyID string
}

type Audit struct {
    CreatedAt time.Time
    CreatedBy string
    UpdatedAt time.Time
    UpdatedBy string
    DeletedAt *time.Time
    DeletedBy string
}

type Entity struct {
    Company
    Audit
}

type Invoice struct {
    Entity
    ID string ` + "`orm:\"pk\"`" + `
    Number string
}
`
	if err := os.WriteFile(filepath.Join(dir, "invoice.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := schema.NewBuilder(dir).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(s.Tables))
	}
	table := s.Tables[0]
	want := map[string]struct {
		scope schema.ScopeKind
		flags func(*schema.Column) bool
	}{
		"company_id": {scope: schema.ScopeCompany, flags: func(c *schema.Column) bool { return c.Scope == schema.ScopeCompany }},
		"created_at": {flags: func(c *schema.Column) bool { return c.CreatedAt }},
		"created_by": {flags: func(c *schema.Column) bool { return c.CreatedBy }},
		"updated_at": {flags: func(c *schema.Column) bool { return c.UpdatedAt }},
		"updated_by": {flags: func(c *schema.Column) bool { return c.UpdatedBy }},
		"deleted_at": {flags: func(c *schema.Column) bool { return c.DeletedAt }},
		"deleted_by": {flags: func(c *schema.Column) bool { return c.DeletedBy }},
		"number":     {},
		"id":         {flags: func(c *schema.Column) bool { return c.PrimaryKey }},
	}
	for name, check := range want {
		var found *schema.Column
		for _, col := range table.Columns {
			if col.Name == name {
				found = col
				break
			}
		}
		if found == nil {
			t.Fatalf("expected column %s", name)
		}
		if check.scope != "" && found.Scope != check.scope {
			t.Fatalf("expected %s scope %s, got %s", name, check.scope, found.Scope)
		}
		if check.flags != nil && !check.flags(found) {
			t.Fatalf("expected %s flags to be set, got %#v", name, found)
		}
	}
}

func TestBuilderFlattensImportedTraits(t *testing.T) {
	dir := t.TempDir()
	src := `package models

import (
    "github.com/dionisius77/dorm/model"
)

type Invoice struct {
    model.Entity
    ID string ` + "`orm:\"pk\"`" + `
    Number string
}
`
	if err := os.WriteFile(filepath.Join(dir, "invoice.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := schema.NewBuilder(dir).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(s.Tables))
	}
	table := s.Tables[0]
	want := map[string]func(*schema.Column) bool{
		"company_id": func(c *schema.Column) bool { return c.Scope == schema.ScopeCompany },
		"created_at": func(c *schema.Column) bool { return c.CreatedAt },
		"created_by": func(c *schema.Column) bool { return c.CreatedBy },
		"updated_at": func(c *schema.Column) bool { return c.UpdatedAt },
		"updated_by": func(c *schema.Column) bool { return c.UpdatedBy },
		"deleted_at": func(c *schema.Column) bool { return c.DeletedAt },
		"deleted_by": func(c *schema.Column) bool { return c.DeletedBy },
		"id":         func(c *schema.Column) bool { return c.PrimaryKey },
		"number":     func(c *schema.Column) bool { return true },
	}
	for name, check := range want {
		var found *schema.Column
		for _, col := range table.Columns {
			if col.Name == name {
				found = col
				break
			}
		}
		if found == nil {
			t.Fatalf("expected column %s", name)
		}
		if !check(found) {
			t.Fatalf("expected %s metadata to be populated, got %#v", name, found)
		}
	}
}
