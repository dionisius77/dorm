package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"

	"github.com/dionisius77/dorm/dialect"
	"github.com/dionisius77/dorm/dialect/postgres"
	driverpostgres "github.com/dionisius77/dorm/driver/postgres"
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
		Inspector:     driverpostgres.New(driverpostgres.Config{DSN: "test", SchemaName: "public"}).Inspector(),
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
	provider := &migrateTestTracerProvider{}
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
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
		Inspector:     driverpostgres.New(driverpostgres.Config{DSN: "test", SchemaName: "public"}).Inspector(),
	})
	_, err := service.Generate(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errkind.ErrUnsupportedFeature) {
		t.Fatalf("expected unsupported feature error, got %T %v", err, err)
	}
	if !migrateSpanErrored(provider.spans, "db.migration.generate") {
		t.Fatalf("expected errored migration span, got %#v", provider.spans)
	}
}

func TestGenerateTracesSchemaBuildAndMigration(t *testing.T) {
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
	provider := &migrateTestTracerProvider{}
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	service := NewService(Config{
		Root:          modelsDir,
		MigrationsDir: filepath.Join(dir, "migrations"),
		SnapshotPath:  filepath.Join(dir, "schemas", "current.snapshot.json"),
		Dialect:       postgres.New(),
		Inspector:     driverpostgres.New(driverpostgres.Config{DSN: "test", SchemaName: "public"}).Inspector(),
	})
	if _, err := service.Generate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"db.schema.build", "db.migration.generate"} {
		if !migrateHasSpan(provider.spans, want) {
			t.Fatalf("expected span %q, got %#v", want, provider.spans)
		}
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
func (failingDialect) RenderSelect(table string, columns []string, distinct bool, joins []string, where []string, groupBy []string, having []string, orderBy []string, limit, offset *int) (string, error) {
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

type migrateTestTracerProvider struct {
	embedded.TracerProvider
	mu    sync.Mutex
	spans []migrateTestSpanRecord
}

type migrateTestSpanRecord struct {
	Name    string
	Errored bool
}

type migrateTestTracer struct {
	embedded.Tracer
	provider *migrateTestTracerProvider
}

type migrateTestSpan struct {
	embedded.Span
	provider *migrateTestTracerProvider
	index    int
}

func (p *migrateTestTracerProvider) Tracer(string, ...oteltrace.TracerOption) oteltrace.Tracer {
	return migrateTestTracer{provider: p}
}

func (t migrateTestTracer) Start(ctx context.Context, name string, _ ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	t.provider.mu.Lock()
	defer t.provider.mu.Unlock()
	t.provider.spans = append(t.provider.spans, migrateTestSpanRecord{Name: name})
	return ctx, migrateTestSpan{provider: t.provider, index: len(t.provider.spans) - 1}
}

func (s migrateTestSpan) End(...oteltrace.SpanEndOption) {}

func (s migrateTestSpan) IsRecording() bool { return true }

func (s migrateTestSpan) RecordError(err error, _ ...oteltrace.EventOption) {
	if err == nil {
		return
	}
	s.provider.mu.Lock()
	if s.index >= 0 && s.index < len(s.provider.spans) {
		s.provider.spans[s.index].Errored = true
	}
	s.provider.mu.Unlock()
}

func (s migrateTestSpan) SpanContext() oteltrace.SpanContext { return oteltrace.SpanContext{} }

func (s migrateTestSpan) SetStatus(_ otelcodes.Code, _ string) {}

func (s migrateTestSpan) SetName(string) {}

func (s migrateTestSpan) SetAttributes(...attribute.KeyValue) {}

func (s migrateTestSpan) AddEvent(string, ...oteltrace.EventOption) {}

func (s migrateTestSpan) AddLink(oteltrace.Link) {}

func (s migrateTestSpan) TracerProvider() oteltrace.TracerProvider { return nil }

func migrateHasSpan(spans []migrateTestSpanRecord, want string) bool {
	for _, span := range spans {
		if span.Name == want {
			return true
		}
	}
	return false
}

func migrateSpanErrored(spans []migrateTestSpanRecord, want string) bool {
	for _, span := range spans {
		if span.Name == want {
			return span.Errored
		}
	}
	return false
}
