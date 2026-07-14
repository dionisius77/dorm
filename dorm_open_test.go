package dorm

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dionisius77/dorm/dialect"
	pgdialect "github.com/dionisius77/dorm/dialect/postgres"
	dormdriver "github.com/dionisius77/dorm/driver"
	"github.com/dionisius77/dorm/orm"
	"github.com/dionisius77/dorm/schema"
)

const openTestSQLDriverName = "dorm-open-preflight"

var (
	openTestDriverOnce sync.Once
	openTestStatesMu   sync.Mutex
	openTestStates     = map[string]*openTestState{}
)

type openTestState struct {
	mu      sync.Mutex
	closed  int
	queries []string
}

type openTestSQLDriver struct{}

type openTestConn struct {
	scenario string
}

type openTestRows struct {
	cols []string
	data [][]sqldriver.Value
	idx  int
}

type openTestResult struct{}

type openTestTracerProvider struct {
	spans int
}

type openTestTracer struct {
	provider *openTestTracerProvider
}

type openTestSpan struct {
	provider *openTestTracerProvider
}

func init() {
	registerOpenTestSQLDriver()
}

func registerOpenTestSQLDriver() {
	openTestDriverOnce.Do(func() {
		sql.Register(openTestSQLDriverName, openTestSQLDriver{})
	})
}

func (openTestSQLDriver) Open(name string) (sqldriver.Conn, error) {
	return &openTestConn{scenario: name}, nil
}

func (c *openTestConn) Prepare(string) (sqldriver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (c *openTestConn) Close() error {
	state := openTestStateFor(c.scenario)
	state.mu.Lock()
	state.closed++
	state.mu.Unlock()
	return nil
}

func (c *openTestConn) Begin() (sqldriver.Tx, error) {
	return nil, fmt.Errorf("transactions not supported")
}

func (c *openTestConn) Ping(context.Context) error { return nil }

func (c *openTestConn) ExecContext(context.Context, string, []sqldriver.NamedValue) (sqldriver.Result, error) {
	return openTestResult{}, nil
}

func (c *openTestConn) QueryContext(_ context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Rows, error) {
	state := openTestStateFor(c.scenario)
	state.mu.Lock()
	state.queries = append(state.queries, query)
	state.mu.Unlock()

	normalized := strings.ToLower(strings.TrimSpace(query))
	switch {
	case strings.Contains(normalized, "from information_schema.tables"):
		return &openTestRows{
			cols: []string{"table_name"},
			data: [][]sqldriver.Value{{"users"}},
		}, nil
	case strings.Contains(normalized, "from information_schema.columns"):
		return &openTestRows{
			cols: []string{"table_name", "column_name", "is_nullable", "data_type", "udt_name", "column_default"},
			data: openTestColumnRows(c.scenario),
		}, nil
	case strings.Contains(normalized, "from pg_constraint"):
		return &openTestRows{
			cols: []string{"table_name", "conname", "contype", "index_name", "columns"},
			data: openTestConstraintRows(c.scenario),
		}, nil
	case strings.Contains(normalized, "from pg_indexes"):
		return &openTestRows{
			cols: []string{"tablename", "indexname", "indexdef"},
		}, nil
	case strings.Contains(normalized, "from orm_migrations"):
		return &openTestRows{
			cols: []string{"name"},
		}, nil
	default:
		return &openTestRows{cols: []string{"value"}}, nil
	}
}

func (c *openTestConn) CheckNamedValue(*sqldriver.NamedValue) error { return nil }

func (openTestResult) LastInsertId() (int64, error) { return 0, nil }

func (openTestResult) RowsAffected() (int64, error) { return 1, nil }

func (p *openTestTracerProvider) Tracer(string) orm.Tracer {
	return openTestTracer{provider: p}
}

func (t openTestTracer) Start(ctx context.Context, name string, _ ...orm.SpanOption) (context.Context, orm.Span) {
	_ = ctx
	_ = name
	t.provider.spans++
	return ctx, openTestSpan{provider: t.provider}
}

func (s openTestSpan) End() {}

func (s openTestSpan) RecordError(error) {}

func (s openTestSpan) SetAttributes(...orm.Attribute) {}

func (r *openTestRows) Columns() []string { return append([]string(nil), r.cols...) }

func (r *openTestRows) Close() error { return nil }

func (r *openTestRows) Next(dest []sqldriver.Value) error {
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

func openTestStateFor(scenario string) *openTestState {
	openTestStatesMu.Lock()
	defer openTestStatesMu.Unlock()
	if state, ok := openTestStates[scenario]; ok {
		return state
	}
	state := &openTestState{}
	openTestStates[scenario] = state
	return state
}

func openTestColumnRows(scenario string) [][]sqldriver.Value {
	switch scenario {
	case "drift":
		return [][]sqldriver.Value{
			{"users", "id", "NO", "bigint", "int8", nil},
		}
	default:
		return [][]sqldriver.Value{
			{"users", "id", "NO", "bigint", "int8", nil},
			{"users", "name", "NO", "text", "text", nil},
		}
	}
}

func openTestConstraintRows(string) [][]sqldriver.Value {
	return [][]sqldriver.Value{
		{"users", "users_pkey", "p", "users_pkey", "id"},
	}
}

type testOpenDriver struct {
	scenario  string
	preflight dormdriver.PreflightConfig
}

func (d testOpenDriver) Validate() error { return nil }

func (d testOpenDriver) Name() string { return "postgres" }

func (d testOpenDriver) Dialect() dialect.Dialect { return pgdialect.New() }

func (d testOpenDriver) Open(context.Context) (*sql.DB, error) {
	return sql.Open(openTestSQLDriverName, d.scenario)
}

func (d testOpenDriver) PreflightConfig() dormdriver.PreflightConfig { return d.preflight }

func writeOpenTestSnapshot(t *testing.T, root string) string {
	t.Helper()
	snapshotPath := filepath.Join(root, "schemas", "current.snapshot.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := schema.NewBuilder(root).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.SaveSnapshot(snapshotPath, &schema.Snapshot{Schema: s}); err != nil {
		t.Fatal(err)
	}
	return snapshotPath
}

func writeOpenTestModel(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `package models

type User struct {
	ID int
	Name string
}
`
	if err := os.WriteFile(filepath.Join(root, "user.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenSkipsPreflightWhenDisabled(t *testing.T) {
	scenario := "disabled"
	state := openTestStateFor(scenario)
	state.mu.Lock()
	state.closed = 0
	state.queries = nil
	state.mu.Unlock()

	db, err := Open(context.Background(), testOpenDriver{
		scenario: scenario,
		preflight: dormdriver.PreflightConfig{
			Enabled:      false,
			Root:         "ignored",
			SnapshotPath: "ignored",
			SchemaName:   "public",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	state.mu.Lock()
	defer state.mu.Unlock()
	for _, q := range state.queries {
		if strings.Contains(strings.ToLower(q), "information_schema") || strings.Contains(strings.ToLower(q), "pg_constraint") {
			t.Fatalf("expected no schema preflight queries, got %q", q)
		}
	}
}

func TestOpenRunsPreflightWhenEnabled(t *testing.T) {
	root := t.TempDir()
	modelRoot := filepath.Join(root, "models")
	writeOpenTestModel(t, modelRoot)
	snapshotPath := writeOpenTestSnapshot(t, modelRoot)

	scenario := "success"
	state := openTestStateFor(scenario)
	state.mu.Lock()
	state.closed = 0
	state.queries = nil
	state.mu.Unlock()

	db, err := Open(context.Background(), testOpenDriver{
		scenario: scenario,
		preflight: dormdriver.PreflightConfig{
			Enabled:      true,
			Root:         modelRoot,
			SnapshotPath: snapshotPath,
			SchemaName:   "public",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.queries) == 0 {
		t.Fatal("expected preflight queries to run")
	}
}

func TestOpenFailsOnPreflightDriftAndClosesDB(t *testing.T) {
	root := t.TempDir()
	modelRoot := filepath.Join(root, "models")
	writeOpenTestModel(t, modelRoot)
	snapshotPath := writeOpenTestSnapshot(t, modelRoot)

	scenario := "drift"
	state := openTestStateFor(scenario)
	state.mu.Lock()
	state.closed = 0
	state.queries = nil
	state.mu.Unlock()

	_, err := Open(context.Background(), testOpenDriver{
		scenario: scenario,
		preflight: dormdriver.PreflightConfig{
			Enabled:      true,
			Root:         modelRoot,
			SnapshotPath: snapshotPath,
			SchemaName:   "public",
		},
	})
	if err == nil {
		t.Fatal("expected preflight drift error")
	}
	if !strings.Contains(err.Error(), "schema preflight detected drift") {
		t.Fatalf("unexpected error: %v", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed == 0 {
		t.Fatal("expected db close on preflight failure")
	}
}

func TestOpenAppliesObservabilityConfig(t *testing.T) {
	scenario := "observability"
	state := openTestStateFor(scenario)
	state.mu.Lock()
	state.closed = 0
	state.queries = nil
	state.mu.Unlock()

	provider := &openTestTracerProvider{}
	db, err := Open(context.Background(), testOpenDriver{
		scenario: scenario,
	}, WithObservability(orm.ObservabilityConfig{
		Tracing:        true,
		TraceSQL:       orm.TraceSQLStatement,
		TracerProvider: provider,
		SQLLogging:     orm.SQLLogDisabled,
		MaskParameters: false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.spans == 0 {
		t.Fatal("expected observability tracer to be used")
	}
}
