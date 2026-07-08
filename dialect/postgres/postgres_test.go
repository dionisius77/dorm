package postgres

import (
	"testing"

	"dorm/schema"
)

func TestRenderSelect(t *testing.T) {
	sql, err := New().RenderSelect("products", []string{`"id"`, `"name"`}, []string{`"company_id" = $1`}, []string{`"name" ASC`}, nil, nil)
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
