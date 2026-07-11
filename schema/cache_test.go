package schema_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dionisius77/dorm/schema"
)

func TestBuilderCacheInvalidatesOnSourceChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "user.go")
	src1 := `package models

type User struct {
    ID string ` + "`orm:\"pk\"`" + `
}
`
	if err := os.WriteFile(path, []byte(src1), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := schema.NewBuilder(dir).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Tables) != 1 || len(first.Tables[0].Columns) != 1 {
		t.Fatalf("unexpected initial schema: %#v", first)
	}

	src2 := `package models

type User struct {
    ID string ` + "`orm:\"pk\"`" + `
    Email string
}
`
	if err := os.WriteFile(path, []byte(src2), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := schema.NewBuilder(dir).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Tables) != 1 || len(second.Tables[0].Columns) != 2 {
		t.Fatalf("expected cache invalidation after source change, got %#v", second)
	}
}

func TestSnapshotCacheInvalidatesOnSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	first := &schema.Snapshot{
		Schema: &schema.Schema{
			Name: "first",
			Tables: []*schema.Table{
				{
					Name: "users",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
					},
				},
			},
		},
	}
	if err := schema.SaveSnapshot(path, first); err != nil {
		t.Fatal(err)
	}
	loaded, err := schema.LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Schema == nil || loaded.Schema.Name != "first" {
		t.Fatalf("unexpected initial snapshot: %#v", loaded)
	}

	second := &schema.Snapshot{
		Schema: &schema.Schema{
			Name: "second",
			Tables: []*schema.Table{
				{
					Name: "users",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "email"},
					},
				},
			},
		},
	}
	if err := schema.SaveSnapshot(path, second); err != nil {
		t.Fatal(err)
	}
	loaded, err = schema.LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Schema == nil || loaded.Schema.Name != "second" {
		t.Fatalf("expected cache invalidation after snapshot save, got %#v", loaded)
	}
	if len(loaded.Schema.Tables[0].Columns) != 2 {
		t.Fatalf("expected updated snapshot columns, got %#v", loaded.Schema.Tables[0].Columns)
	}
}
