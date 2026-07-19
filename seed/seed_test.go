package seed

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/orm"
	"github.com/dionisius77/dorm/schema"
)

var registerSeedTestDriverOnce sync.Once

type seedTestDriver struct{}

type seedTestConn struct{}

type seedTestTx struct{}

type seedTestRows struct {
	cols []string
	data [][]driver.Value
	idx  int
}

type seedTestResult struct{}

var (
	seedTestMu            sync.Mutex
	seedTestQueries       []string
	seedTestQueryRows     [][]driver.Value
	seedTestBeginCount    int
	seedTestCommitCount   int
	seedTestRollbackCount int
)

func init() {
	registerSeedTestDriverOnce.Do(func() {
		sql.Register("seed-test", seedTestDriver{})
	})
}

func (seedTestDriver) Open(string) (driver.Conn, error) { return seedTestConn{}, nil }

func (seedTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (seedTestConn) Close() error { return nil }

func (seedTestConn) Begin() (driver.Tx, error) { return seedTestTx{}, nil }

func (seedTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return seedTestTx{}, nil
}

func (seedTestConn) Ping(context.Context) error { return nil }

func (seedTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	seedTestStoreQuery(query)
	return seedTestResult{}, nil
}

func (seedTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	seedTestStoreQuery(query)
	seedTestMu.Lock()
	defer seedTestMu.Unlock()
	rows := make([][]driver.Value, 0, len(seedTestQueryRows))
	for _, row := range seedTestQueryRows {
		rows = append(rows, append([]driver.Value(nil), row...))
	}
	return &seedTestRows{
		cols: []string{"id", "code", "name", "created_at", "updated_at"},
		data: rows,
	}, nil
}

func (seedTestConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (seedTestTx) Commit() error {
	seedTestMu.Lock()
	seedTestCommitCount++
	seedTestMu.Unlock()
	return nil
}

func (seedTestTx) Rollback() error {
	seedTestMu.Lock()
	seedTestRollbackCount++
	seedTestMu.Unlock()
	return nil
}

func (seedTestResult) LastInsertId() (int64, error) { return 0, nil }

func (seedTestResult) RowsAffected() (int64, error) { return 1, nil }

func (r *seedTestRows) Columns() []string { return append([]string(nil), r.cols...) }

func (r *seedTestRows) Close() error { return nil }

func (r *seedTestRows) Next(dest []driver.Value) error {
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

func seedTestStoreQuery(query string) {
	seedTestMu.Lock()
	defer seedTestMu.Unlock()
	seedTestQueries = append(seedTestQueries, query)
}

func seedTestReset(rows ...[]driver.Value) {
	seedTestMu.Lock()
	defer seedTestMu.Unlock()
	seedTestQueries = nil
	seedTestQueryRows = rows
	seedTestBeginCount = 0
	seedTestCommitCount = 0
	seedTestRollbackCount = 0
}

func seedTestLastQuery() string {
	seedTestMu.Lock()
	defer seedTestMu.Unlock()
	if len(seedTestQueries) == 0 {
		return ""
	}
	return seedTestQueries[len(seedTestQueries)-1]
}

func TestSyncCreatesMissingRecord(t *testing.T) {
	seedTestReset()
	provider := &seedTestTracerProvider{}
	dbConn, err := sql.Open("seed-test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	db := orm.New(orm.Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "roles",
					GoTypeName: "Role",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "code", Unique: true},
						{Name: "name"},
						{Name: "created_at", CreatedAt: true},
						{Name: "updated_at", UpdatedAt: true},
					},
				},
			},
		},
		Observability: orm.ObservabilityConfig{
			Tracing:        true,
			TracerProvider: provider,
		},
	})

	type Role struct {
		ID        int
		Code      string
		Name      string
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	if err := Sync(context.Background(), db, &Role{ID: 1, Code: "ADMIN", Name: "Administrator"}, Key("Code")); err != nil {
		t.Fatal(err)
	}
	query := strings.ToLower(seedTestLastQuery())
	if !strings.Contains(query, `insert into "roles"`) {
		t.Fatalf("expected insert query, got %q", query)
	}
	if !seedHasSpan(provider.spans, "seed.sync") {
		t.Fatalf("expected seed sync span, got %#v", provider.spans)
	}
	if got := seedSpanInt64Attr(provider, "seed.sync", "seed.records_inserted"); got != 1 {
		t.Fatalf("expected one inserted record, got %#v", got)
	}
	if got := seedSpanAttr(provider, "seed.sync", "seed.model"); got != "Role" {
		t.Fatalf("expected model attr, got %#v", got)
	}
}

func TestSyncUpdatesExistingRecord(t *testing.T) {
	seedTestReset([][]driver.Value{
		{int64(1), "ADMIN", "Old", time.Now().UTC(), time.Now().UTC()},
	}[0])
	provider := &seedTestTracerProvider{}
	dbConn, err := sql.Open("seed-test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	db := orm.New(orm.Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "roles",
					GoTypeName: "Role",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "code", Unique: true},
						{Name: "name"},
						{Name: "created_at", CreatedAt: true},
						{Name: "updated_at", UpdatedAt: true},
					},
				},
			},
		},
		Observability: orm.ObservabilityConfig{
			Tracing:        true,
			TracerProvider: provider,
		},
	})

	type Role struct {
		ID        int
		Code      string
		Name      string
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	if err := Sync(context.Background(), db, &Role{ID: 1, Code: "ADMIN", Name: "Administrator"}, Key("Code")); err != nil {
		t.Fatal(err)
	}
	query := strings.ToLower(seedTestLastQuery())
	if !strings.Contains(query, `update "roles" set`) {
		t.Fatalf("expected update query, got %q", query)
	}
	if got := seedSpanInt64Attr(provider, "seed.sync", "seed.records_updated"); got != 1 {
		t.Fatalf("expected one updated record, got %#v", got)
	}
}

func TestSyncSkipsUnchangedRecord(t *testing.T) {
	seedTestReset([][]driver.Value{
		{int64(1), "ADMIN", "Administrator", time.Now().UTC(), time.Now().UTC()},
	}[0])
	provider := &seedTestTracerProvider{}
	dbConn, err := sql.Open("seed-test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	db := orm.New(orm.Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "roles",
					GoTypeName: "Role",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "code", Unique: true},
						{Name: "name"},
						{Name: "created_at", CreatedAt: true},
						{Name: "updated_at", UpdatedAt: true},
					},
				},
			},
		},
		Observability: orm.ObservabilityConfig{
			Tracing:        true,
			TracerProvider: provider,
		},
	})

	type Role struct {
		ID        int
		Code      string
		Name      string
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	if err := Sync(context.Background(), db, &Role{ID: 1, Code: "ADMIN", Name: "Administrator"}, Key("Code")); err != nil {
		t.Fatal(err)
	}
	if got := seedSpanInt64Attr(provider, "seed.sync", "seed.records_skipped"); got != 1 {
		t.Fatalf("expected one skipped record, got %#v spans=%#v", got, provider.spans)
	}
}

func TestSyncRecordsErrorOnConflict(t *testing.T) {
	seedTestReset(
		[]driver.Value{int64(1), "ADMIN", "Old", time.Now().UTC(), time.Now().UTC()},
		[]driver.Value{int64(2), "ADMIN", "Older", time.Now().UTC(), time.Now().UTC()},
	)
	provider := &seedTestTracerProvider{}
	dbConn, err := sql.Open("seed-test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	db := orm.New(orm.Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "roles",
					GoTypeName: "Role",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "code", Unique: true},
						{Name: "name"},
						{Name: "created_at", CreatedAt: true},
						{Name: "updated_at", UpdatedAt: true},
					},
				},
			},
		},
		Observability: orm.ObservabilityConfig{
			Tracing:        true,
			TracerProvider: provider,
		},
	})

	type Role struct {
		ID        int
		Code      string
		Name      string
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	err = Sync(context.Background(), db, &Role{Code: "ADMIN", Name: "Administrator"}, Key("Code"))
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !seedSpanErrored(provider, "seed.sync") {
		t.Fatalf("expected errored seed span, got %#v", provider.spans)
	}
}

func TestRunOrdersDependencies(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	var order []string
	Register(
		recordingSeeder{name: "permissions", deps: []string{"roles"}, order: &order},
		recordingSeeder{name: "roles", order: &order},
	)

	dbConn, err := sql.Open("seed-test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	db := orm.New(orm.Config{DB: dbConn, Dialect: postgres.New()})
	if err := Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(order, ","), "roles,permissions"; got != want {
		t.Fatalf("expected dependency order %q, got %q", want, got)
	}
}

func TestRunRejectsCycles(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	Register(
		recordingSeeder{name: "a", deps: []string{"b"}},
		recordingSeeder{name: "b", deps: []string{"a"}},
	)

	dbConn, err := sql.Open("seed-test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	db := orm.New(orm.Config{DB: dbConn, Dialect: postgres.New()})
	err = Run(context.Background(), db)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errkind.Is(err, errkind.KindConfiguration) {
		t.Fatalf("expected configuration error, got %T %v", err, err)
	}
}

func TestRunTracesSeedOperation(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	order := []string{}
	Register(
		recordingSeeder{name: "roles", order: &order},
		recordingSeeder{name: "permissions", deps: []string{"roles"}, order: &order},
	)
	provider := &seedTestTracerProvider{}
	dbConn, err := sql.Open("seed-test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()
	db := orm.New(orm.Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Observability: orm.ObservabilityConfig{
			Tracing:        true,
			TracerProvider: provider,
		},
	})
	ctx := context.WithValue(context.Background(), seedTestContextKey{}, "trace-seed")
	if err := Run(ctx, db, WithSingleTransaction()); err != nil {
		t.Fatal(err)
	}
	if !seedHasSpan(provider.spans, "seed.run") {
		t.Fatalf("expected seed.run span, got %#v", provider.spans)
	}
	if got := seedSpanAttr(provider, "seed.run", "seed.operation"); got != "run" {
		t.Fatalf("expected run operation attr, got %#v", got)
	}
	if got := seedSpanInt64Attr(provider, "seed.run", "seed.models_processed"); got != 2 {
		t.Fatalf("expected two processed models, got %#v", got)
	}
	if got := seedSpanContext(provider, "seed.run"); got != "trace-seed" {
		t.Fatalf("expected context propagation, got %#v", got)
	}
}

func TestSeedValuesEqualIgnoresAuditFields(t *testing.T) {
	type Role struct {
		ID        int
		Code      string
		Name      string
		CreatedAt time.Time
		UpdatedAt time.Time
	}
	left := Role{
		ID:        1,
		Code:      "ADMIN",
		Name:      "Administrator",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	right := Role{
		ID:   1,
		Code: "ADMIN",
		Name: "Administrator",
	}
	if !seedValuesEqual(reflect.ValueOf(left), reflect.ValueOf(right)) {
		t.Fatal("expected audit fields to be ignored")
	}
}

type recordingSeeder struct {
	name  string
	deps  []string
	order *[]string
}

func (s recordingSeeder) Name() string { return s.name }

func (s recordingSeeder) Dependencies() []string { return append([]string(nil), s.deps...) }

func (s recordingSeeder) Run(_ context.Context, _ *orm.Session) error {
	*s.order = append(*s.order, s.name)
	return nil
}

type seedTestContextKey struct{}

type seedTestTracerProvider struct {
	mu    sync.Mutex
	spans []seedTestSpanRecord
}

type seedTestSpanRecord struct {
	Name       string
	Attributes map[string]any
	Errored    bool
	Context    string
}

type seedTestTracer struct {
	provider *seedTestTracerProvider
}

type seedTestSpan struct {
	provider *seedTestTracerProvider
	index    int
}

func (p *seedTestTracerProvider) Tracer(string) orm.Tracer {
	return seedTestTracer{provider: p}
}

func (t seedTestTracer) Start(ctx context.Context, name string, _ ...orm.SpanOption) (context.Context, orm.Span) {
	t.provider.mu.Lock()
	defer t.provider.mu.Unlock()
	rec := seedTestSpanRecord{Name: name, Attributes: map[string]any{}}
	if v, ok := ctx.Value(seedTestContextKey{}).(string); ok {
		rec.Context = v
	}
	t.provider.spans = append(t.provider.spans, rec)
	return ctx, seedTestSpan{provider: t.provider, index: len(t.provider.spans) - 1}
}

func (s seedTestSpan) End() {}

func (s seedTestSpan) RecordError(err error) {
	if err == nil {
		return
	}
	s.provider.mu.Lock()
	if s.index >= 0 && s.index < len(s.provider.spans) {
		s.provider.spans[s.index].Errored = true
	}
	s.provider.mu.Unlock()
}

func (s seedTestSpan) SetAttributes(attrs ...orm.Attribute) {
	s.provider.mu.Lock()
	defer s.provider.mu.Unlock()
	if s.index < 0 || s.index >= len(s.provider.spans) {
		return
	}
	rec := s.provider.spans[s.index]
	if rec.Attributes == nil {
		rec.Attributes = map[string]any{}
	}
	for _, attr := range attrs {
		rec.Attributes[attr.Key] = attr.Value
	}
	s.provider.spans[s.index] = rec
}

func seedHasSpan(spans []seedTestSpanRecord, name string) bool {
	for _, span := range spans {
		if span.Name == name {
			return true
		}
	}
	return false
}

func seedSpanAttr(provider *seedTestTracerProvider, spanName, key string) any {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	for _, span := range provider.spans {
		if span.Name != spanName {
			continue
		}
		return span.Attributes[key]
	}
	return nil
}

func seedSpanInt64Attr(provider *seedTestTracerProvider, spanName, key string) int64 {
	v := seedSpanAttr(provider, spanName, key)
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func seedSpanErrored(provider *seedTestTracerProvider, spanName string) bool {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	for _, span := range provider.spans {
		if span.Name == spanName {
			return span.Errored
		}
	}
	return false
}

func seedSpanContext(provider *seedTestTracerProvider, spanName string) string {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	for _, span := range provider.spans {
		if span.Name == spanName {
			return span.Context
		}
	}
	return ""
}
