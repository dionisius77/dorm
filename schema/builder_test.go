package schema_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"dorm/schema"
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
}
