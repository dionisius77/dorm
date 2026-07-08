package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/schema"
)

func TestGenerateWritesDeterministicMigration(t *testing.T) {
	dir := t.TempDir()
	modelsDir := filepath.Join(dir, "models")
	migrationsDir := filepath.Join(dir, "migrations")
	snapshotPath := filepath.Join(dir, "schemas", "current.snapshot.json")
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
	service := NewService(Config{
		Root:          modelsDir,
		MigrationsDir: migrationsDir,
		SnapshotPath:  snapshotPath,
		Dialect:       postgres.New(),
		Inspector:     schema.PostgresInspector{},
	})
	result, err := service.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.UpSQL) == 0 {
		t.Fatalf("expected migration SQL")
	}
	if err := service.Write(result); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("expected snapshot: %v", err)
	}
}
