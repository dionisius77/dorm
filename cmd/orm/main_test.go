package main

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dionisius77/dorm/schema"
)

var fakePG = &fakePostgresDriver{}
var registerFakePGOnce sync.Once

type fakePostgresDriver struct {
	mu      sync.Mutex
	applied []string
}

type fakeConn struct{}

type fakeRows struct {
	cols []string
	data [][]driver.Value
	idx  int
}

type fakeResult struct{}

func init() {
	registerFakePGOnce.Do(func() {
		sql.Register("postgres", fakePG)
	})
}

func (d *fakePostgresDriver) Open(name string) (driver.Conn, error) {
	_ = name
	return fakeConn{}, nil
}

func (c fakeConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported: %s", query)
}

func (c fakeConn) Close() error { return nil }

func (c fakeConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("transactions not supported") }

func (c fakeConn) Ping(ctx context.Context) error {
	_ = ctx
	return nil
}

func (c fakeConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	_ = ctx
	_ = query
	_ = args
	return fakeResult{}, nil
}

func (c fakeConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	_ = ctx
	_ = args
	normalized := strings.ToLower(strings.TrimSpace(query))
	switch {
	case strings.Contains(normalized, "from orm_migrations"):
		fakePG.mu.Lock()
		defer fakePG.mu.Unlock()
		rows := make([][]driver.Value, 0, len(fakePG.applied))
		for _, name := range fakePG.applied {
			rows = append(rows, []driver.Value{name})
		}
		return &fakeRows{cols: []string{"name"}, data: rows}, nil
	case strings.Contains(normalized, "information_schema.tables"):
		return &fakeRows{cols: []string{"table_name"}}, nil
	case strings.Contains(normalized, "information_schema.columns"):
		return &fakeRows{cols: []string{"table_name", "column_name", "is_nullable", "data_type", "udt_name", "column_default"}}, nil
	case strings.Contains(normalized, "from pg_constraint"):
		return &fakeRows{cols: []string{"table_name", "conname", "contype", "index_name", "columns"}}, nil
	case strings.Contains(normalized, "pg_indexes"):
		return &fakeRows{cols: []string{"tablename", "indexname", "indexdef"}}, nil
	default:
		return &fakeRows{cols: []string{"value"}}, nil
	}
}

func (c fakeConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }

func (fakeResult) RowsAffected() (int64, error) { return 1, nil }

func (r *fakeRows) Columns() []string { return append([]string(nil), r.cols...) }

func (r *fakeRows) Close() error { return nil }

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.data) {
		return io.EOF
	}
	for i := range dest {
		if i < len(r.data[r.idx]) {
			dest[i] = r.data[r.idx][i]
		} else {
			dest[i] = nil
		}
	}
	r.idx++
	return nil
}

func TestInitCreatesProjectStructure(t *testing.T) {
	root := t.TempDir()
	if err := cmdInit([]string{"--root", root}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "migrations"),
		filepath.Join(root, "schemas"),
		filepath.Join(root, "models"),
		filepath.Join(root, "orm.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}

func TestMigrateStatusReportsAppliedAndPending(t *testing.T) {
	root := t.TempDir()
	migrationsDir := filepath.Join(root, "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(migrationsDir, "0001_schema.up.sql"), []byte("select 1;"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(migrationsDir, "0002_schema.up.sql"), []byte("select 2;"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(filepath.Join(root, "orm.json"), cliConfig{
		Root:          root,
		ModelsDir:     "models",
		MigrationsDir: "migrations",
		SchemasDir:    "schemas",
		SnapshotPath:  filepath.Join("schemas", "current.snapshot.json"),
		SchemaName:    "public",
		Driver:        "postgres",
		DSN:           "status",
		ConfigFile:    "orm.json",
	}); err != nil {
		t.Fatal(err)
	}
	fakePG.mu.Lock()
	fakePG.applied = []string{"0001_schema"}
	fakePG.mu.Unlock()

	out, err := captureStdout(func() error {
		return cmdMigrateStatus(context.Background(), []string{"--root", root})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "applied:") || !strings.Contains(out, "- 0001_schema") {
		t.Fatalf("expected applied migration in output, got %q", out)
	}
	if !strings.Contains(out, "pending:") || !strings.Contains(out, "- 0002_schema") {
		t.Fatalf("expected pending migration in output, got %q", out)
	}
}

func TestSchemaCheckDetectsDrift(t *testing.T) {
	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modelSource := []byte(`package models

type Widget struct {
	ID   int
	Name string
}
`)
	if err := os.WriteFile(filepath.Join(modelsDir, "widget.go"), modelSource, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(filepath.Join(root, "orm.json"), cliConfig{
		Root:          root,
		ModelsDir:     "models",
		MigrationsDir: "migrations",
		SchemasDir:    "schemas",
		SnapshotPath:  filepath.Join("schemas", "current.snapshot.json"),
		SchemaName:    "public",
		Driver:        "postgres",
		DSN:           "schema-check",
		ConfigFile:    "orm.json",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(func() error {
		return cmdSchemaCheck(context.Background(), []string{"--root", root})
	})
	if err == nil {
		t.Fatal("expected schema drift error")
	}
	if !strings.Contains(err.Error(), "schema drift detected") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Schema Drift Detected") {
		t.Fatalf("expected drift message, got %q", out)
	}
}

func TestDoctorValidatesSnapshotConnectivityAndDialect(t *testing.T) {
	root := t.TempDir()
	snapshotPath := filepath.Join(root, "schemas", "current.snapshot.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := schema.SaveSnapshot(snapshotPath, &schema.Snapshot{
		GeneratedAt: time.Now().UTC(),
		Schema:      &schema.Schema{Name: "public"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(filepath.Join(root, "orm.json"), cliConfig{
		Root:          root,
		ModelsDir:     "models",
		MigrationsDir: "migrations",
		SchemasDir:    "schemas",
		SnapshotPath:  filepath.Join("schemas", "current.snapshot.json"),
		SchemaName:    "public",
		Driver:        "postgres",
		DSN:           "doctor",
		ConfigFile:    "orm.json",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(func() error {
		return cmdDoctor(context.Background(), []string{"--root", root})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"doctor: compatibility ok",
		"doctor: config ok",
		"doctor: connectivity ok",
		"doctor: snapshot ok",
		"doctor: dialect ok",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func captureStdout(fn func() error) (string, error) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- fn()
		_ = w.Close()
	}()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	err = <-errCh
	_ = r.Close()
	return buf.String(), err
}
