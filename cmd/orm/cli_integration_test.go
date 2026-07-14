package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/internal/itest"
	"github.com/dionisius77/dorm/migrate"
	"github.com/dionisius77/dorm/schema"
)

func TestCLIIntegrationCommands(t *testing.T) {
	project := itest.NewProject(t)
	project.WriteFile(t, "models/core.go", itest.DefaultModelSource)
	project.WriteJSON(t, "orm.json", map[string]any{
		"root":           ".",
		"models_dir":     "models",
		"migrations_dir": "migrations",
		"schemas_dir":    "schemas",
		"snapshot_path":  "schemas/current.snapshot.json",
		"schema_name":    project.Schema,
		"driver":         "postgres",
		"dsn":            project.DSN,
		"config_file":    "orm.json",
	})

	service := migrate.NewService(migrate.Config{
		Root:          project.ModelsDir,
		MigrationsDir: project.MigrationsDir,
		SnapshotPath:  project.SnapshotPath,
		SchemaName:    project.Schema,
		Dialect:       postgres.New(),
		Inspector:     schema.PostgresInspector{},
	})
	result, err := service.Generate(context.Background())
	if err != nil {
		t.Fatalf("generate migration: %v", err)
	}
	if result == nil || result.Diff == nil || result.Diff.Empty() {
		t.Fatal("expected generated migration diff")
	}
	if err := service.Write(result); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	bin := buildORMBinary(t)

	out, code := runBinary(t, bin, "migrate", "run", "--root", project.Root)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", code, out)
	}
	if !strings.Contains(out, "✓ Migration completed") {
		t.Fatalf("expected migration run success, got %q", out)
	}

	filesBefore, err := filepath.Glob(filepath.Join(project.MigrationsDir, "*.sql"))
	if err != nil {
		t.Fatalf("glob migrations before noop generate: %v", err)
	}
	out, code = runBinary(t, bin, "migrate", "generate", "--root", project.Root)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", code, out)
	}
	if !strings.Contains(out, "✓ No schema changes detected.") {
		t.Fatalf("expected no-op migration generate, got %q", out)
	}
	filesAfter, err := filepath.Glob(filepath.Join(project.MigrationsDir, "*.sql"))
	if err != nil {
		t.Fatalf("glob migrations after noop generate: %v", err)
	}
	if len(filesBefore) != len(filesAfter) {
		t.Fatalf("expected no new migration files, before=%d after=%d", len(filesBefore), len(filesAfter))
	}

	out, code = runBinary(t, bin, "schema", "check", "--root", project.Root)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", code, out)
	}
	if !strings.Contains(out, "✓ Schema is up to date") {
		t.Fatalf("expected schema check success, got %q", out)
	}

	out, code = runBinary(t, bin, "seed", "run", "--root", project.Root)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", code, out)
	}
	if !strings.Contains(out, "✓ Seed run completed") {
		t.Fatalf("expected seed run success, got %q", out)
	}

	out, code = runBinary(t, bin, "migrate", "rollback", "--name", result.MigrationName, "--root", project.Root)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", code, out)
	}
	if !strings.Contains(out, fmt.Sprintf("✓ Rolled back %s", result.MigrationName)) {
		t.Fatalf("expected rollback success, got %q", out)
	}

	sqlDB := project.OpenSQL(t)
	defer sqlDB.Close()
	var tableCount int
	if err := sqlDB.QueryRowContext(context.Background(), `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_name IN ('products', 'roles')
	`, project.Schema).Scan(&tableCount); err != nil {
		t.Fatalf("check tables after cli rollback: %v", err)
	}
	if tableCount != 0 {
		t.Fatalf("expected cli rollback to remove generated tables, found %d", tableCount)
	}
}

func buildORMBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "orm")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/orm")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build orm binary: %v\n%s", err, string(output))
	}
	return bin
}

func runBinary(t *testing.T, bin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(output), exitErr.ExitCode()
	}
	t.Fatalf("run orm binary: %v", err)
	return "", -1
}
