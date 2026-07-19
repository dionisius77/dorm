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

	"go.opentelemetry.io/otel"
	otelattribute "go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"

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
	mu        sync.Mutex
	spans     int
	spanNames []string
	attrs     [][]orm.Attribute
	ctxVals   []string
}

type openTestTracer struct {
	provider *openTestTracerProvider
}

type openTestSpan struct {
	provider *openTestTracerProvider
	index    int
}

type openTestContextKey struct{}

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

func (c *openTestConn) Ping(ctx context.Context) error { return ctx.Err() }

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
	t.provider.mu.Lock()
	t.provider.spans++
	t.provider.spanNames = append(t.provider.spanNames, name)
	t.provider.attrs = append(t.provider.attrs, nil)
	if v, ok := ctx.Value(openTestContextKey{}).(string); ok {
		t.provider.ctxVals = append(t.provider.ctxVals, v)
	} else {
		t.provider.ctxVals = append(t.provider.ctxVals, "")
	}
	idx := len(t.provider.spanNames) - 1
	t.provider.mu.Unlock()
	return ctx, openTestSpan{provider: t.provider, index: idx}
}

func (s openTestSpan) End() {}

func (s openTestSpan) RecordError(error) {}

func (s openTestSpan) SetAttributes(attrs ...orm.Attribute) {
	s.provider.mu.Lock()
	defer s.provider.mu.Unlock()
	if s.index < 0 || s.index >= len(s.provider.attrs) {
		return
	}
	s.provider.attrs[s.index] = append(s.provider.attrs[s.index], attrs...)
}

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
	inspector schema.Inspector
}

func (d testOpenDriver) Validate() error { return nil }

func (d testOpenDriver) Name() string { return "postgres" }

func (d testOpenDriver) Dialect() dialect.Dialect { return pgdialect.New() }

func (d testOpenDriver) Open(context.Context) (*sql.DB, error) {
	return sql.Open(openTestSQLDriverName, d.scenario)
}

func (d testOpenDriver) PreflightConfig() dormdriver.PreflightConfig { return d.preflight }

func (d testOpenDriver) Inspector() schema.Inspector { return d.inspector }

func (d testOpenDriver) ConnectionInfo() dormdriver.ConnectionInfo {
	return dormdriver.ConnectionInfo{
		System:        "postgresql",
		Name:          d.scenario,
		Namespace:     "public",
		ServerAddress: "localhost",
		ServerPort:    5432,
	}
}

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

func TestRegisterDriverStoresRegisteredDriver(t *testing.T) {
	prev := RegisteredDriver()
	t.Cleanup(func() { RegisterDriver(prev) })

	drv := testOpenDriver{scenario: "registered"}
	RegisterDriver(drv)
	if got := RegisteredDriver(); got != drv {
		t.Fatalf("expected registered driver to be returned")
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
	provider := &openTestTracerProvider{}
	schemaProvider := &openOtelTracerProvider{}
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(schemaProvider)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	db, err := Open(context.Background(), testOpenDriver{
		scenario: scenario,
		preflight: dormdriver.PreflightConfig{
			Enabled:      true,
			Root:         modelRoot,
			SnapshotPath: snapshotPath,
			SchemaName:   "public",
		},
		inspector: schema.PostgresInspector{},
	}, WithObservability(orm.ObservabilityConfig{
		Tracing:        true,
		TracerProvider: provider,
		SQLLogging:     orm.SQLLogDisabled,
		TraceSQL:       orm.TraceSQLDisabled,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, want := range []string{"db.connect", "db.ping"} {
		if !containsOpenSpan(provider.spanNames, want) {
			t.Fatalf("expected span %q in %v", want, provider.spanNames)
		}
	}
	if !hasOpenSpanAttribute(provider.attrs, "db.connect", "driver.name", "postgres") {
		t.Fatalf("expected driver metadata on connect span, got %#v", provider.attrs)
	}
	for _, want := range []string{"db.schema.check", "db.schema.build", "db.schema.inspect"} {
		if !containsOpenSpan(schemaProvider.spanNames, want) {
			t.Fatalf("expected schema span %q in %v", want, schemaProvider.spanNames)
		}
	}
}

func TestOpenFailsOnPreflightDriftAndClosesDB(t *testing.T) {
	root := t.TempDir()
	modelRoot := filepath.Join(root, "models")
	writeOpenTestModel(t, modelRoot)
	snapshotPath := writeOpenTestSnapshot(t, modelRoot)
	snap, err := schema.LoadSnapshot(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	drift := snap.Schema.Clone()
	if len(drift.Tables) > 0 {
		drift.Tables[0].Columns = drift.Tables[0].Columns[:1]
	}

	scenario := "drift"
	state := openTestStateFor(scenario)
	state.mu.Lock()
	state.closed = 0
	state.queries = nil
	state.mu.Unlock()

	_, openErr := Open(context.Background(), testOpenDriver{
		scenario: scenario,
		preflight: dormdriver.PreflightConfig{
			Enabled:      true,
			Root:         modelRoot,
			SnapshotPath: snapshotPath,
			SchemaName:   "public",
		},
		inspector: &staticSchemaInspector{schema: drift},
	})
	if openErr == nil {
		t.Fatal("expected preflight drift error")
	}
	if !strings.Contains(openErr.Error(), "schema preflight detected drift") {
		t.Fatalf("unexpected error: %v", openErr)
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

func TestOpenSkipsSpansWhenObservabilityDisabled(t *testing.T) {
	provider := &openTestTracerProvider{}
	db, err := Open(context.Background(), testOpenDriver{
		scenario: "disabled-spans",
	}, WithObservability(orm.ObservabilityConfig{
		Tracing:        false,
		TracerProvider: provider,
		SQLLogging:     orm.SQLLogDisabled,
		TraceSQL:       orm.TraceSQLDisabled,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if provider.spans != 0 {
		t.Fatalf("expected no spans when tracing is disabled, got %#v", provider.spanNames)
	}
}

type staticSchemaInspector struct {
	schema *schema.Schema
	calls  int
}

func (i *staticSchemaInspector) Inspect(context.Context, *sql.DB, string) (*schema.Schema, error) {
	i.calls++
	if i.schema == nil {
		return nil, fmt.Errorf("nil schema")
	}
	return i.schema.Clone(), nil
}

func TestOpenResolvesInspectorFromDriver(t *testing.T) {
	root := t.TempDir()
	modelRoot := filepath.Join(root, "models")
	writeOpenTestModel(t, modelRoot)
	snapshotPath := writeOpenTestSnapshot(t, modelRoot)
	snap, err := schema.LoadSnapshot(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	inspector := &staticSchemaInspector{schema: snap.Schema}
	db, err := Open(context.Background(), testOpenDriver{
		scenario: "inspector",
		preflight: dormdriver.PreflightConfig{
			Enabled:      true,
			Root:         modelRoot,
			SnapshotPath: snapshotPath,
			SchemaName:   "public",
		},
		inspector: inspector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if inspector.calls == 0 {
		t.Fatal("expected driver inspector to be used during preflight")
	}
}

func TestOpenPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Open(ctx, testOpenDriver{
		scenario: "canceled",
	})
	if err == nil {
		t.Fatal("expected canceled context error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context cancellation to propagate, got %v", err)
	}
}

func TestOpenEmitsConnectionLifecycleSpans(t *testing.T) {
	provider := &openTestTracerProvider{}
	ctx := context.WithValue(context.Background(), openTestContextKey{}, "trace-root")
	db, err := Open(ctx, testOpenDriver{
		scenario: "trace",
	}, WithObservability(orm.ObservabilityConfig{
		Tracing:        true,
		TracerProvider: provider,
		SQLLogging:     orm.SQLLogDisabled,
		TraceSQL:       orm.TraceSQLDisabled,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	for _, want := range []string{"db.connect", "db.ping", "db.close"} {
		found := false
		for _, got := range provider.spanNames {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected span %q in %v", want, provider.spanNames)
		}
	}
	for i, got := range provider.ctxVals {
		if got != "trace-root" {
			t.Fatalf("expected span %d to inherit context value, got %q", i, got)
		}
	}
	if len(provider.attrs) == 0 {
		t.Fatal("expected connection span attributes")
	}
	attrs := map[string]any{}
	for _, attr := range provider.attrs[0] {
		attrs[attr.Key] = attr.Value
	}
	for _, want := range []string{"db.system", "db.name", "db.namespace", "server.address", "server.port"} {
		if _, ok := attrs[want]; !ok {
			t.Fatalf("expected %s attribute in connection span, got %#v", want, attrs)
		}
	}
}

func containsOpenSpan(names []string, want string) bool {
	for _, got := range names {
		if got == want {
			return true
		}
	}
	return false
}

func hasOpenSpanAttribute(attrs [][]orm.Attribute, spanName, key string, value any) bool {
	for i, attrSet := range attrs {
		if i >= len(attrSet) {
			continue
		}
		_ = spanName
		for _, attr := range attrSet {
			if attr.Key == key && attr.Value == value {
				return true
			}
		}
	}
	return false
}

type openOtelTracerProvider struct {
	embedded.TracerProvider
	mu        sync.Mutex
	spanNames []string
}

type openOtelTracer struct {
	embedded.Tracer
	provider *openOtelTracerProvider
}

type openOtelSpan struct {
	embedded.Span
}

func (p *openOtelTracerProvider) Tracer(string, ...oteltrace.TracerOption) oteltrace.Tracer {
	return openOtelTracer{provider: p}
}

func (t openOtelTracer) Start(ctx context.Context, name string, _ ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	t.provider.mu.Lock()
	t.provider.spanNames = append(t.provider.spanNames, name)
	t.provider.mu.Unlock()
	return ctx, openOtelSpan{}
}

func (openOtelSpan) End(...oteltrace.SpanEndOption) {}
func (openOtelSpan) IsRecording() bool { return false }
func (openOtelSpan) RecordError(error, ...oteltrace.EventOption) {}
func (openOtelSpan) SpanContext() oteltrace.SpanContext { return oteltrace.SpanContext{} }
func (openOtelSpan) SetStatus(otelcodes.Code, string) {}
func (openOtelSpan) SetName(string) {}
func (openOtelSpan) SetAttributes(...otelattribute.KeyValue) {}
func (openOtelSpan) AddEvent(string, ...oteltrace.EventOption) {}
func (openOtelSpan) AddLink(oteltrace.Link) {}
func (openOtelSpan) TracerProvider() oteltrace.TracerProvider { return nil }
