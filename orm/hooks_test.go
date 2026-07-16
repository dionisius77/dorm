package orm

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/schema"
)

const hookTestDriverName = "orm-hooks"

var hookTestDriverOnce sync.Once

type hookTestContextKey struct{}

type hookCall struct {
	Name      string
	CompanyID string
	Policy    string
	TraceID   string
	Tx        bool
}

type hookTestState struct {
	mu            sync.Mutex
	beginCount    int
	commitCount   int
	rollbackCount int
	queries       []string
	execs         []string
	calls         []hookCall
	commitErr     error
	rollbackErr   error
}

type hookTestDriver struct{}

type hookTestConn struct {
	scenario string
}

type hookTestTx struct {
	state *hookTestState
}

type hookTestRows struct {
	cols []string
	data [][]sqldriver.Value
	idx  int
}

type hookTestResult struct{}

type hookTraceProvider struct {
	mu    sync.Mutex
	spans []hookTraceSpanRecord
}

type hookTraceSpanRecord struct {
	Name    string
	Events  []string
	Errored bool
}

type hookTraceTracer struct {
	provider *hookTraceProvider
}

type hookTraceSpan struct {
	provider *hookTraceProvider
	index    int
}

func init() {
	hookTestDriverOnce.Do(func() {
		sql.Register(hookTestDriverName, hookTestDriver{})
	})
}

func (hookTestDriver) Open(name string) (sqldriver.Conn, error) {
	return &hookTestConn{scenario: name}, nil
}

func (c *hookTestConn) state() *hookTestState {
	return hookTestStateFor(c.scenario)
}

func (c *hookTestConn) Prepare(string) (sqldriver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (c *hookTestConn) Close() error { return nil }

func (c *hookTestConn) Begin() (sqldriver.Tx, error) {
	return c.BeginTx(context.Background(), sqldriver.TxOptions{})
}

func (c *hookTestConn) BeginTx(ctx context.Context, _ sqldriver.TxOptions) (sqldriver.Tx, error) {
	state := c.state()
	state.mu.Lock()
	state.beginCount++
	state.mu.Unlock()
	_ = ctx
	return &hookTestTx{state: state}, nil
}

func (c *hookTestConn) Ping(context.Context) error { return nil }

func (c *hookTestConn) ExecContext(_ context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Result, error) {
	state := c.state()
	state.mu.Lock()
	state.execs = append(state.execs, query)
	state.mu.Unlock()
	return hookTestResult{}, nil
}

func (c *hookTestConn) QueryContext(ctx context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Rows, error) {
	state := c.state()
	state.mu.Lock()
	state.queries = append(state.queries, query)
	state.mu.Unlock()
	_ = ctx
	return &hookTestRows{
		cols: []string{"id", "company_id", "name", "deleted_at", "created_at", "updated_at"},
		data: [][]sqldriver.Value{{
			"1",
			"company-a",
			"alpha",
			nil,
			time.Unix(0, 0).UTC(),
			time.Unix(0, 0).UTC(),
		}},
	}, nil
}

func (c *hookTestConn) CheckNamedValue(*sqldriver.NamedValue) error { return nil }

func (r *hookTestRows) Columns() []string { return append([]string(nil), r.cols...) }

func (r *hookTestRows) Close() error { return nil }

func (r *hookTestRows) Next(dest []sqldriver.Value) error {
	if r.idx >= len(r.data) {
		return io.EOF
	}
	for i := range dest {
		if i < len(r.data[r.idx]) {
			dest[i] = r.data[r.idx][i]
		}
	}
	r.idx++
	return nil
}

func (r hookTestResult) LastInsertId() (int64, error) { return 0, nil }

func (r hookTestResult) RowsAffected() (int64, error) { return 1, nil }

func (t *hookTestTx) Commit() error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	t.state.commitCount++
	if t.state.commitErr != nil {
		return t.state.commitErr
	}
	return nil
}

func (t *hookTestTx) Rollback() error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	t.state.rollbackCount++
	if t.state.rollbackErr != nil {
		return t.state.rollbackErr
	}
	return nil
}

func (p *hookTraceProvider) Tracer(string) Tracer {
	return hookTraceTracer{provider: p}
}

func (t hookTraceTracer) Start(ctx context.Context, name string, _ ...SpanOption) (context.Context, Span) {
	t.provider.mu.Lock()
	t.provider.spans = append(t.provider.spans, hookTraceSpanRecord{Name: name})
	idx := len(t.provider.spans) - 1
	t.provider.mu.Unlock()
	return ctx, hookTraceSpan{provider: t.provider, index: idx}
}

func (s hookTraceSpan) End() {}

func (s hookTraceSpan) RecordError(error) {
	s.provider.mu.Lock()
	if s.index >= 0 && s.index < len(s.provider.spans) {
		s.provider.spans[s.index].Errored = true
	}
	s.provider.mu.Unlock()
}

func (s hookTraceSpan) SetAttributes(...Attribute) {}

func (s hookTraceSpan) AddEvent(name string, _ ...Attribute) {
	s.provider.mu.Lock()
	if s.index >= 0 && s.index < len(s.provider.spans) {
		s.provider.spans[s.index].Events = append(s.provider.spans[s.index].Events, name)
	}
	s.provider.mu.Unlock()
}

func hookTestStateFor(scenario string) *hookTestState {
	hookTestStateMu.Lock()
	defer hookTestStateMu.Unlock()
	if state, ok := hookTestStates[scenario]; ok {
		return state
	}
	state := &hookTestState{}
	hookTestStates[scenario] = state
	return state
}

var (
	hookTestStateMu sync.Mutex
	hookTestStates  = map[string]*hookTestState{}
)

func hookTestDB(t *testing.T, scenario string, obs ObservabilityConfig) (*DB, *hookTestState) {
	t.Helper()
	dbConn, err := sql.Open(hookTestDriverName, scenario)
	if err != nil {
		t.Fatalf("open hook test db: %v", err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})
	db := New(Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "hook_models",
					GoTypeName: "hookModel",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "company_id", Scope: schema.ScopeCompany},
						{Name: "name"},
						{Name: "created_at", CreatedAt: true},
						{Name: "updated_at", UpdatedAt: true},
						{Name: "deleted_at", SoftDelete: true},
					},
				},
			},
		},
		Observability: obs,
	})
	return db, hookTestStateFor(scenario)
}

func hookResetState(scenario string) {
	hookTestStateMu.Lock()
	defer hookTestStateMu.Unlock()
	hookTestStates[scenario] = &hookTestState{}
}

func hookRecorderEntries(state *hookTestState) []hookCall {
	state.mu.Lock()
	defer state.mu.Unlock()
	return append([]hookCall(nil), state.calls...)
}

func hookLastQuery(state *hookTestState) string {
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.queries) == 0 {
		return ""
	}
	return state.queries[len(state.queries)-1]
}

type hookRecorder struct {
	state *hookTestState
}

var hookTestRecorderInstance *hookRecorder

func (r *hookRecorder) add(name string, ctx context.Context, tx *DB) {
	if r == nil || r.state == nil {
		return
	}
	ac, _ := access.FromContext(ctx)
	call := hookCall{
		Name:      name,
		CompanyID: fmt.Sprint(ac.CompanyID),
		Policy:    access.PolicyFromContext(ctx).Name(),
		Tx:        tx != nil && tx.tx != nil,
	}
	if v, ok := ctx.Value(hookTestContextKey{}).(string); ok {
		call.TraceID = v
	}
	r.state.mu.Lock()
	r.state.calls = append(r.state.calls, call)
	r.state.mu.Unlock()
}

type hookModel struct {
	Recorder  *hookRecorder
	ID        string `orm:"pk"`
	CompanyID string `orm:"company"`
	Name      string
	CreatedAt time.Time  `orm:"created_at"`
	UpdatedAt time.Time  `orm:"updated_at"`
	DeletedAt *time.Time `orm:"soft_delete"`
	FailOn    string
}

var hookFailure = errors.New("hook failure")

func (m *hookModel) BeforeCreate(ctx context.Context, tx *DB) error {
	if hookTestRecorderInstance != nil {
		hookTestRecorderInstance.add("BeforeCreate", ctx, tx)
	} else if m != nil && m.Recorder != nil {
		m.Recorder.add("BeforeCreate", ctx, tx)
	}
	if m != nil && m.FailOn == "BeforeCreate" {
		return hookFailure
	}
	return nil
}

func (m *hookModel) AfterCreate(ctx context.Context, tx *DB) error {
	if hookTestRecorderInstance != nil {
		hookTestRecorderInstance.add("AfterCreate", ctx, tx)
	} else if m != nil && m.Recorder != nil {
		m.Recorder.add("AfterCreate", ctx, tx)
	}
	if m != nil && m.FailOn == "AfterCreate" {
		return hookFailure
	}
	return nil
}

func (m *hookModel) BeforeUpdate(ctx context.Context, tx *DB) error {
	if hookTestRecorderInstance != nil {
		hookTestRecorderInstance.add("BeforeUpdate", ctx, tx)
	} else if m != nil && m.Recorder != nil {
		m.Recorder.add("BeforeUpdate", ctx, tx)
	}
	if m != nil && m.FailOn == "BeforeUpdate" {
		return hookFailure
	}
	return nil
}

func (m *hookModel) AfterUpdate(ctx context.Context, tx *DB) error {
	if hookTestRecorderInstance != nil {
		hookTestRecorderInstance.add("AfterUpdate", ctx, tx)
	} else if m != nil && m.Recorder != nil {
		m.Recorder.add("AfterUpdate", ctx, tx)
	}
	if m != nil && m.FailOn == "AfterUpdate" {
		return hookFailure
	}
	return nil
}

func (m *hookModel) BeforeDelete(ctx context.Context, tx *DB) error {
	if hookTestRecorderInstance != nil {
		hookTestRecorderInstance.add("BeforeDelete", ctx, tx)
	} else if m != nil && m.Recorder != nil {
		m.Recorder.add("BeforeDelete", ctx, tx)
	}
	if m != nil && m.FailOn == "BeforeDelete" {
		return hookFailure
	}
	return nil
}

func (m *hookModel) AfterDelete(ctx context.Context, tx *DB) error {
	if hookTestRecorderInstance != nil {
		hookTestRecorderInstance.add("AfterDelete", ctx, tx)
	} else if m != nil && m.Recorder != nil {
		m.Recorder.add("AfterDelete", ctx, tx)
	}
	if m != nil && m.FailOn == "AfterDelete" {
		return hookFailure
	}
	return nil
}

func (m *hookModel) AfterFind(ctx context.Context, tx *DB) error {
	if hookTestRecorderInstance != nil {
		hookTestRecorderInstance.add("AfterFind", ctx, tx)
	} else if m != nil && m.Recorder != nil {
		m.Recorder.add("AfterFind", ctx, tx)
	}
	if m != nil && m.FailOn == "AfterFind" {
		return hookFailure
	}
	return nil
}

func TestLifecycleHooksSuccess(t *testing.T) {
	scenario := t.Name()
	hookResetState(scenario)
	provider := &hookTraceProvider{}
	db, state := hookTestDB(t, scenario, ObservabilityConfig{
		Tracing:        true,
		TracerProvider: provider,
	})
	recorder := &hookRecorder{state: state}
	hookTestRecorderInstance = recorder
	t.Cleanup(func() {
		hookTestRecorderInstance = nil
	})
	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	}), access.Default())
	ctx = context.WithValue(ctx, hookTestContextKey{}, "trace-123")

	err := db.Transaction(ctx, func(tx *DB) error {
		model := &hookModel{
			Recorder: recorder,
			ID:       "1",
			Name:     "alpha",
		}
		if err := tx.Create(model); err != nil {
			return err
		}
		var rows []hookModel
		if err := tx.Find(&rows, Where("id = ?", "1")); err != nil {
			return err
		}
		model.Name = "beta"
		if err := tx.Update(model); err != nil {
			return err
		}
		if err := tx.Delete(model); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction with hooks: %v", err)
	}

	calls := hookRecorderEntries(state)
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Name)
		if call.CompanyID != "company-a" {
			t.Fatalf("expected company context in hook, got %#v", call)
		}
		if call.Policy != access.Default().Name() {
			t.Fatalf("expected default policy in hook, got %#v", call)
		}
		if call.TraceID != "trace-123" {
			t.Fatalf("expected request context in hook, got %#v", call)
		}
		if !call.Tx {
			t.Fatalf("expected transaction handle in hook, got %#v", call)
		}
	}

	expected := []string{
		"BeforeCreate",
		"AfterCreate",
		"AfterFind",
		"BeforeUpdate",
		"AfterUpdate",
		"BeforeDelete",
		"AfterDelete",
	}
	if !equalStrings(names, expected) {
		t.Fatalf("unexpected hook order: got %v want %v", names, expected)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	var sawHookSpan, sawEvent bool
	for _, span := range provider.spans {
		if strings.HasPrefix(span.Name, "db.hook.") {
			sawHookSpan = true
		}
		for _, event := range span.Events {
			if event == "BeforeCreate" || event == "AfterFind" {
				sawEvent = true
			}
		}
	}
	if !sawHookSpan || !sawEvent {
		t.Fatalf("expected hook spans and events, got %#v", provider.spans)
	}
}

func TestLifecycleHookFailureRollsBackTransaction(t *testing.T) {
	scenario := t.Name()
	hookResetState(scenario)
	db, state := hookTestDB(t, scenario, ObservabilityConfig{})
	recorder := &hookRecorder{state: state}
	hookTestRecorderInstance = recorder
	t.Cleanup(func() {
		hookTestRecorderInstance = nil
	})
	ctx := access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	})

	err := db.Transaction(ctx, func(tx *DB) error {
		return tx.Create(&hookModel{
			Recorder: recorder,
			ID:       "1",
			Name:     "fail",
			FailOn:   "BeforeCreate",
		})
	})
	if err == nil {
		t.Fatal("expected hook failure")
	}
	if !errors.Is(err, hookFailure) {
		t.Fatalf("expected hook failure cause, got %v", err)
	}
	if !strings.Contains(err.Error(), "BeforeCreate") {
		t.Fatalf("expected lifecycle context in error, got %v", err)
	}

	if state.beginCount != 1 || state.commitCount != 0 || state.rollbackCount != 1 {
		t.Fatalf("expected rollback on hook failure, got %+v", state)
	}

	calls := hookRecorderEntries(state)
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Name)
	}
	if !equalStrings(names, []string{"BeforeCreate"}) {
		t.Fatalf("expected only before create hook, got %v", names)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
