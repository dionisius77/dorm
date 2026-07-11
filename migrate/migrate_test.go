package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dionisius77/dorm/dialect"
	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/errkind"
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

func TestGenerateReturnsUnsupportedFeatureError(t *testing.T) {
	dir := t.TempDir()
	modelsDir := filepath.Join(dir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `package models

type User struct {
    ID string ` + "`orm:\"pk\"`" + `
}
`
	if err := os.WriteFile(filepath.Join(modelsDir, "user.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(Config{
		Root:          modelsDir,
		MigrationsDir: filepath.Join(dir, "migrations"),
		SnapshotPath:  filepath.Join(dir, "schemas", "current.snapshot.json"),
		Dialect:       failingDialect{},
		Inspector:     schema.PostgresInspector{},
	})
	_, err := service.Generate(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errkind.ErrUnsupportedFeature) {
		t.Fatalf("expected unsupported feature error, got %T %v", err, err)
	}
}

type failingDialect struct{}

func (failingDialect) Name() string                                     { return "failing" }
func (failingDialect) QuoteIdent(s string) string                       { return s }
func (failingDialect) Placeholder(i int) string                         { return fmt.Sprintf("$%d", i) }
func (failingDialect) Capabilities() dialect.Capabilities               { return dialect.Capabilities{} }
func (failingDialect) ColumnDefinition(*schema.Column) (string, error)  { return "", nil }
func (failingDialect) RenderOperation(schema.Operation) (string, error) { return "", nil }
func (failingDialect) RenderMigration(*schema.Diff) ([]string, error) {
	return nil, fmt.Errorf("render migration unsupported")
}
func (failingDialect) RenderSelect(table string, columns []string, where []string, orderBy []string, limit, offset *int) (string, error) {
	return "", nil
}
func (failingDialect) RenderInsert(table string, columns []string, returning []string) (string, error) {
	return "", nil
}
func (failingDialect) RenderUpdate(table string, set []string, where []string, returning []string) (string, error) {
	return "", nil
}
func (failingDialect) RenderDelete(table string, where []string, returning []string) (string, error) {
	return "", nil
}
