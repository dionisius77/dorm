package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/dionisius77/dorm/dialect"
	"github.com/dionisius77/dorm/errkind"
	dormerrors "github.com/dionisius77/dorm/errors"
	"github.com/dionisius77/dorm/schema"
)

const rawTestDriverName = "orm-raw"

var rawTestDriverOnce sync.Once

type rawTestState struct {
	mu           sync.Mutex
	queryCount   int
	execCount    int
	lastQuery    string
	lastArgs     []any
	queryRows    [][]driver.Value
	queryErr     error
	execErr      error
	rowsAffected int64
}

type rawTestDriver struct{}
type rawTestConn struct {
	scenario string
}
type rawTestTx struct {
	state *rawTestState
}
type rawTestRows struct {
	state *rawTestState
	cols  []string
	idx   int
}
type rawTestResult struct {
	rowsAffected int64
}

type rawTestTracerProvider struct {
	mu    sync.Mutex
	spans []rawTestSpanRecord
}

type rawTestSpanRecord struct {
	Name       string
	Attributes map[string]any
	Errored    bool
}

type rawTestTracer struct {
	provider *rawTestTracerProvider
}

type rawTestSpan struct {
	provider *rawTestTracerProvider
	index    int
}

type rawTestPlaceholderDialect struct{}

func init() {
	rawTestDriverOnce.Do(func() {
		sql.Register(rawTestDriverName, rawTestDriver{})
	})
}

func (rawTestDriver) Open(name string) (driver.Conn, error) {
	return &rawTestConn{scenario: name}, nil
}

func (c *rawTestConn) state() *rawTestState {
	return rawTestStateFor(c.scenario)
}

func (c *rawTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (c *rawTestConn) Close() error { return nil }

func (c *rawTestConn) Begin() (driver.Tx, error) {
	return &rawTestTx{state: c.state()}, nil
}

func (c *rawTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &rawTestTx{state: c.state()}, nil
}

func (c *rawTestConn) Ping(context.Context) error { return nil }

func (c *rawTestConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	state := c.state()
	state.mu.Lock()
	state.execCount++
	state.lastQuery = query
	state.lastArgs = namedValuesToAny(args)
	err := state.execErr
	rowsAffected := state.rowsAffected
	state.mu.Unlock()
	if err != nil {
		return nil, err
	}
	_ = ctx
	return rawTestResult{rowsAffected: rowsAffected}, nil
}

func (c *rawTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	state := c.state()
	state.mu.Lock()
	state.queryCount++
	state.lastQuery = query
	state.lastArgs = namedValuesToAny(args)
	err := state.queryErr
	state.mu.Unlock()
	if err != nil {
		return nil, err
	}
	_ = ctx
	return &rawTestRows{
		state: state,
		cols:  []string{"id", "email"},
	}, nil
}

func (c *rawTestConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (t *rawTestTx) Commit() error   { return nil }
func (t *rawTestTx) Rollback() error { return nil }

func (r *rawTestRows) Columns() []string { return append([]string(nil), r.cols...) }

func (r *rawTestRows) Close() error { return nil }

func (r *rawTestRows) Next(dest []driver.Value) error {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	if r.idx >= len(r.state.queryRows) {
		return io.EOF
	}
	row := r.state.queryRows[r.idx]
	r.idx++
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		} else {
			dest[i] = nil
		}
	}
	return nil
}

func (r rawTestResult) LastInsertId() (int64, error) { return 0, nil }

func (r rawTestResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

func (p *rawTestTracerProvider) Tracer(string) Tracer {
	return rawTestTracer{provider: p}
}

func (t rawTestTracer) Start(ctx context.Context, name string, _ ...SpanOption) (context.Context, Span) {
	t.provider.mu.Lock()
	t.provider.spans = append(t.provider.spans, rawTestSpanRecord{
		Name:       name,
		Attributes: map[string]any{},
	})
	idx := len(t.provider.spans) - 1
	t.provider.mu.Unlock()
	return ctx, rawTestSpan{provider: t.provider, index: idx}
}

func (s rawTestSpan) End() {}

func (s rawTestSpan) RecordError(err error) {
	s.provider.mu.Lock()
	if s.index >= 0 && s.index < len(s.provider.spans) {
		s.provider.spans[s.index].Errored = err != nil
	}
	s.provider.mu.Unlock()
}

func (s rawTestSpan) SetAttributes(attrs ...Attribute) {
	s.provider.mu.Lock()
	defer s.provider.mu.Unlock()
	if s.index < 0 || s.index >= len(s.provider.spans) {
		return
	}
	record := s.provider.spans[s.index]
	if record.Attributes == nil {
		record.Attributes = map[string]any{}
	}
	for _, attr := range attrs {
		record.Attributes[attr.Key] = attr.Value
	}
	s.provider.spans[s.index] = record
}

func rawTestStateFor(scenario string) *rawTestState {
	rawTestStateMu.Lock()
	defer rawTestStateMu.Unlock()
	if state, ok := rawTestStates[scenario]; ok {
		return state
	}
	state := &rawTestState{}
	rawTestStates[scenario] = state
	return state
}

var (
	rawTestStateMu sync.Mutex
	rawTestStates  = map[string]*rawTestState{}
)

func rawTestResetState(scenario string) {
	rawTestStateMu.Lock()
	defer rawTestStateMu.Unlock()
	rawTestStates[scenario] = &rawTestState{
		rowsAffected: 1,
		queryRows:    [][]driver.Value{{"user-1", "alice@example.com"}},
	}
}

func rawTestDB(t *testing.T, scenario string, obs ObservabilityConfig) (*DB, *rawTestState) {
	t.Helper()
	dbConn, err := sql.Open(rawTestDriverName, scenario)
	if err != nil {
		t.Fatalf("open raw test db: %v", err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})
	db := New(Config{
		DB:      dbConn,
		Dialect: rawTestPlaceholderDialect{},
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "users",
					GoTypeName: "rawTestUser",
					Columns: []*schema.Column{
						{Name: "id"},
						{Name: "email"},
					},
				},
			},
		},
		Observability: obs,
	})
	return db, rawTestStateFor(scenario)
}

func namedValuesToAny(args []driver.NamedValue) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, len(args))
	for i, arg := range args {
		out[i] = arg.Value
	}
	return out
}

func (rawTestPlaceholderDialect) Name() string               { return "raw-test" }
func (rawTestPlaceholderDialect) QuoteIdent(s string) string { return s }
func (rawTestPlaceholderDialect) Placeholder(i int) string   { return "@" + itoa(i) }
func (rawTestPlaceholderDialect) Capabilities() dialect.Capabilities {
	return dialect.Capabilities{}
}
func (rawTestPlaceholderDialect) ColumnDefinition(*schema.Column) (string, error)  { return "", nil }
func (rawTestPlaceholderDialect) RenderOperation(schema.Operation) (string, error) { return "", nil }
func (rawTestPlaceholderDialect) RenderMigration(*schema.Diff) ([]string, error)   { return nil, nil }
func (rawTestPlaceholderDialect) RenderSelect(string, []string, []string, []string, *int, *int) (string, error) {
	return "", nil
}
func (rawTestPlaceholderDialect) RenderInsert(string, []string, []string) (string, error) {
	return "", nil
}
func (rawTestPlaceholderDialect) RenderUpdate(string, []string, []string, []string) (string, error) {
	return "", nil
}
func (rawTestPlaceholderDialect) RenderDelete(string, []string, []string) (string, error) {
	return "", nil
}

func TestRawSQLRequiresExplicitPolicyDecision(t *testing.T) {
	scenario := t.Name()
	rawTestResetState(scenario)
	db, state := rawTestDB(t, scenario, ObservabilityConfig{})

	var users []struct {
		ID    string
		Email string
	}
	err := db.Raw(context.Background(), "SELECT id, email FROM users WHERE email = ?", "alice@example.com").Scan(&users)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, dormerrors.ErrRawSQLPolicyRequired) {
		t.Fatalf("expected raw SQL policy sentinel, got %T %v", err, err)
	}
	var policyErr *dormerrors.RawSQLPolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected typed raw SQL policy error, got %T %v", err, err)
	}
	if policyErr == nil || !strings.Contains(policyErr.Error(), "WithoutPolicy") {
		t.Fatalf("expected policy hint, got %#v", policyErr)
	}

	_, err = db.Raw(context.Background(), "UPDATE users SET email = ? WHERE id = ?", "alice@example.com", "user-1").Exec()
	if err == nil {
		t.Fatal("expected exec error")
	}
	if !errors.Is(err, dormerrors.ErrRawSQLPolicyRequired) {
		t.Fatalf("expected raw SQL policy sentinel for exec, got %T %v", err, err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.queryCount != 0 || state.execCount != 0 {
		t.Fatalf("expected no driver execution without policy, got query=%d exec=%d", state.queryCount, state.execCount)
	}
}

func TestRawSQLScanAndExecBindPlaceholders(t *testing.T) {
	scenario := t.Name()
	rawTestResetState(scenario)
	db, state := rawTestDB(t, scenario, ObservabilityConfig{})

	type rawUser struct {
		ID    string
		Email string
	}
	var users []rawUser
	err := db.Raw(context.Background(), "SELECT id, email FROM users WHERE email = ? AND id = ?", "alice@example.com", "user-1").
		WithoutPolicy().
		Scan(&users)
	if err != nil {
		t.Fatalf("scan raw sql: %v", err)
	}
	if len(users) != 1 || users[0].ID != "user-1" || users[0].Email != "alice@example.com" {
		t.Fatalf("unexpected scan result: %#v", users)
	}

	_, err = db.Raw(context.Background(), "UPDATE users SET email = ? WHERE id = ?", "alice+1@example.com", "user-1").
		WithoutPolicy().
		Exec()
	if err != nil {
		t.Fatalf("exec raw sql: %v", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if !strings.Contains(state.lastQuery, "email = @1") || !strings.Contains(state.lastQuery, "id = @2") {
		t.Fatalf("expected rebinding of placeholders, got %q", state.lastQuery)
	}
	if state.queryCount != 1 || state.execCount != 1 {
		t.Fatalf("unexpected driver counts: query=%d exec=%d", state.queryCount, state.execCount)
	}
	if len(state.lastArgs) != 2 || state.lastArgs[0] != "alice+1@example.com" || state.lastArgs[1] != "user-1" {
		t.Fatalf("unexpected bound args: %#v", state.lastArgs)
	}
}

func TestRawSQLPreservesWrappedDriverErrors(t *testing.T) {
	scenario := t.Name()
	rawTestResetState(scenario)
	state := rawTestStateFor(scenario)
	state.mu.Lock()
	state.execErr = errors.New("exec boom")
	state.queryErr = errors.New("query boom")
	state.mu.Unlock()

	db, _ := rawTestDB(t, scenario, ObservabilityConfig{})

	_, err := db.Raw(context.Background(), "UPDATE users SET email = ? WHERE id = ?", "alice@example.com", "user-1").
		WithoutPolicy().
		Exec()
	if err == nil {
		t.Fatal("expected exec error")
	}
	if !errors.Is(err, errkind.ErrRuntimeQuery) {
		t.Fatalf("expected runtime query sentinel, got %T %v", err, err)
	}
	if !errors.Is(err, state.execErr) {
		t.Fatalf("expected underlying exec error to be preserved, got %v", err)
	}

	var users []struct {
		ID    string
		Email string
	}
	err = db.Raw(context.Background(), "SELECT id, email FROM users WHERE email = ?", "alice@example.com").
		WithoutPolicy().
		Scan(&users)
	if err == nil {
		t.Fatal("expected query error")
	}
	if !errors.Is(err, errkind.ErrRuntimeQuery) {
		t.Fatalf("expected runtime query sentinel, got %T %v", err, err)
	}
	if !errors.Is(err, state.queryErr) {
		t.Fatalf("expected underlying query error to be preserved, got %v", err)
	}
}

func TestRawSQLEmitsObservabilitySpans(t *testing.T) {
	scenario := t.Name()
	rawTestResetState(scenario)
	provider := &rawTestTracerProvider{}
	db, state := rawTestDB(t, scenario, ObservabilityConfig{
		Tracing:        true,
		TraceSQL:       TraceSQLStatement,
		TracerProvider: provider,
	})

	type rawUser struct {
		ID    string
		Email string
	}
	var users []rawUser
	if err := db.Raw(context.Background(), "SELECT id, email FROM users WHERE email = ?", "alice@example.com").
		WithoutPolicy().
		Scan(&users); err != nil {
		t.Fatalf("scan raw sql: %v", err)
	}
	if _, err := db.Raw(context.Background(), "UPDATE users SET email = ? WHERE id = ?", "alice+1@example.com", "user-1").
		WithoutPolicy().
		Exec(); err != nil {
		t.Fatalf("exec raw sql: %v", err)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	var sawScan, sawExec bool
	for _, span := range provider.spans {
		switch span.Name {
		case "db.raw.scan":
			sawScan = true
			if got := span.Attributes["orm.raw"]; got != true {
				t.Fatalf("expected raw span attribute, got %#v", span.Attributes)
			}
			if got := span.Attributes["orm.operation"]; got != "raw_scan" {
				t.Fatalf("expected raw scan operation, got %#v", span.Attributes)
			}
			if got := span.Attributes["db.statement"]; got == "" {
				t.Fatalf("expected statement visibility, got %#v", span.Attributes)
			}
			if got := span.Attributes["orm.rows"]; got != int64(1) {
				t.Fatalf("expected one scanned row, got %#v", span.Attributes)
			}
		case "db.raw.exec":
			sawExec = true
			if got := span.Attributes["orm.raw"]; got != true {
				t.Fatalf("expected raw span attribute, got %#v", span.Attributes)
			}
			if got := span.Attributes["orm.operation"]; got != "raw_exec" {
				t.Fatalf("expected raw exec operation, got %#v", span.Attributes)
			}
			if got := span.Attributes["db.statement"]; got == "" {
				t.Fatalf("expected statement visibility, got %#v", span.Attributes)
			}
			if got := span.Attributes["orm.rows"]; got != int64(1) {
				t.Fatalf("expected affected rows, got %#v", span.Attributes)
			}
		}
	}
	if !sawScan || !sawExec {
		t.Fatalf("expected raw spans, got %#v", provider.spans)
	}
	state.mu.Lock()
	if state.queryCount != 1 || state.execCount != 1 {
		t.Fatalf("unexpected driver counts: query=%d exec=%d", state.queryCount, state.execCount)
	}
	state.mu.Unlock()
}
