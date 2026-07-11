package schema_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/dionisius77/dorm/schema"
)

type fakeInspector struct {
	actual *schema.Schema
}

func (f fakeInspector) Inspect(ctx context.Context, db *sql.DB, schemaName string) (*schema.Schema, error) {
	_ = ctx
	_ = db
	_ = schemaName
	return f.actual.Clone(), nil
}

func TestDetectDriftReportsSchemaAndSnapshotDiff(t *testing.T) {
	expected := &schema.Schema{
		Name: "products",
		Tables: []*schema.Table{
			{
				Name: "products",
				Columns: []*schema.Column{
					{Name: "id", Type: schema.Type{Name: "uuid", Kind: schema.TypeUUID}, PrimaryKey: true},
					{Name: "name", Type: schema.Type{Name: "text", Kind: schema.TypeString}},
				},
			},
		},
	}
	snapshot := &schema.Snapshot{
		Schema: &schema.Schema{
			Name: "products",
			Tables: []*schema.Table{
				{
					Name: "products",
					Columns: []*schema.Column{
						{Name: "id", Type: schema.Type{Name: "uuid", Kind: schema.TypeUUID}, PrimaryKey: true},
					},
				},
			},
		},
	}
	actual := &schema.Schema{
		Name: "products",
		Tables: []*schema.Table{
			{
				Name: "products",
				Columns: []*schema.Column{
					{Name: "id", Type: schema.Type{Name: "uuid", Kind: schema.TypeUUID}, PrimaryKey: true},
					{Name: "full_name", Type: schema.Type{Name: "text", Kind: schema.TypeString}},
				},
			},
		},
	}

	report, err := schema.DetectDriftWithSnapshot(expected, snapshot, actual)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasDrift() {
		t.Fatalf("expected drift report")
	}
	if !report.HasSnapshotDrift() {
		t.Fatalf("expected snapshot drift report")
	}
}

func TestDetectDriftFromSourceUsesBuilderAndInspector(t *testing.T) {
	dir := t.TempDir()
	modelsDir := filepath.Join(dir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `package models

type User struct {
    ID string ` + "`orm:\"pk\"`" + `
    Email string ` + "`orm:\"unique\"`" + `
}
`
	if err := os.WriteFile(filepath.Join(modelsDir, "user.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := &schema.Snapshot{
		Schema: &schema.Schema{
			Name: "models",
			Tables: []*schema.Table{
				{
					Name: "users",
					Columns: []*schema.Column{
						{Name: "id", Type: schema.Type{Name: "uuid", Kind: schema.TypeUUID}, PrimaryKey: true},
					},
				},
			},
		},
	}
	snapshotPath := filepath.Join(dir, "snapshot.json")
	if err := schema.SaveSnapshot(snapshotPath, snapshot); err != nil {
		t.Fatal(err)
	}
	actual := &schema.Schema{
		Name: "models",
		Tables: []*schema.Table{
			{
				Name: "users",
				Columns: []*schema.Column{
					{Name: "id", Type: schema.Type{Name: "uuid", Kind: schema.TypeUUID}, PrimaryKey: true},
					{Name: "email", Type: schema.Type{Name: "text", Kind: schema.TypeString}},
				},
			},
		},
	}

	report, err := schema.DetectDriftFromSource(context.Background(), modelsDir, fakeInspector{actual: actual}, nil, "public", snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasDrift() {
		t.Fatalf("expected model drift")
	}
	if !report.HasSnapshotDrift() {
		t.Fatalf("expected snapshot drift")
	}
	if report.Expected == nil || report.Actual == nil || report.Snapshot == nil {
		t.Fatalf("expected report to include expected, actual, and snapshot schema")
	}
}
