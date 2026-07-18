package postgres

import (
	"testing"

	"github.com/dionisius77/dorm/schema"
)

func TestRenderSelect(t *testing.T) {
	sql, err := New().RenderSelect("products", []string{`"id"`, `"name"`}, false, nil, []string{`"company_id" = $1`}, nil, nil, []string{`"name" ASC`}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sql == "" {
		t.Fatalf("expected sql")
	}
}

func TestColumnDefinition(t *testing.T) {
	sql, err := New().ColumnDefinition(&schema.Column{Name: "id", Type: schema.Type{Name: "uuid", Kind: schema.TypeUUID}, PrimaryKey: true})
	if err != nil {
		t.Fatal(err)
	}
	if sql == "" {
		t.Fatalf("expected column definition")
	}
}

func TestRenderView(t *testing.T) {
	sql, err := New().RenderView(&schema.View{
		Name: "active_users",
		SQL:  "SELECT * FROM users WHERE deleted_at IS NULL",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sql == "" {
		t.Fatalf("expected view sql")
	}
}

func TestRenderMaterializedView(t *testing.T) {
	sql, err := New().RenderView(&schema.View{
		Name:         "active_users_mv",
		Materialized: true,
		SQL:          "SELECT * FROM users",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sql == "" {
		t.Fatalf("expected materialized view sql")
	}
}

func TestRenderExpressionPredicateAndDefault(t *testing.T) {
	d := New()
	if sql, err := d.RenderExpression("LOWER(email)"); err != nil || sql == "" {
		t.Fatalf("expected expression sql, got %q err=%v", sql, err)
	}
	if sql, err := d.RenderPredicate(`"email"`, "=", "$1"); err != nil || sql == "" {
		t.Fatalf("expected predicate sql, got %q err=%v", sql, err)
	}
	if sql, err := d.RenderDefault("now()"); err != nil || sql == "" {
		t.Fatalf("expected default sql, got %q err=%v", sql, err)
	}
}
