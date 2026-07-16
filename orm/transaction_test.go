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

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/dialect/postgres"
	dormerrors "github.com/dionisius77/dorm/errors"
	"github.com/dionisius77/dorm/schema"
)

const transactionTestDriverName = "orm-transaction"

var transactionTestDriverOnce sync.Once

type transactionTestContextKey struct{}

type transactionTestState struct {
	mu            sync.Mutex
	beginCount    int
	commitCount   int
	rollbackCount int
	savepoints    []string
	queries       []string
	beginContext  []any
	execContext   []any
	queryContext  []any
	commitErr     error
	rollbackErr   error
}

type transactionTestDriver struct{}

type transactionTestConn struct {
	scenario string
}

type transactionTestTx struct {
	scenario string
	state    *transactionTestState
}

type transactionTestRows struct {
	cols []string
	idx  int
}

type transactionTestResult struct{}

type transactionTestTracerProvider struct {
	mu    sync.Mutex
	spans []transactionTestSpanRecord
}

type transactionTestSpanRecord struct {
	Name    string
	Errored bool
}

type transactionTestTracer struct {
	provider *transactionTestTracerProvider
}

type transactionTestSpan struct {
	provider *transactionTestTracerProvider
	index    int
}

func init() {
	transactionTestDriverOnce.Do(func() {
		sql.Register(transactionTestDriverName, transactionTestDriver{})
	})
}

func (transactionTestDriver) Open(name string) (sqldriver.Conn, error) {
	return &transactionTestConn{scenario: name}, nil
}

func (c *transactionTestConn) state() *transactionTestState {
	return transactionTestStateFor(c.scenario)
}

func (c *transactionTestConn) Prepare(string) (sqldriver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (c *transactionTestConn) Close() error { return nil }

func (c *transactionTestConn) Begin() (sqldriver.Tx, error) {
	return c.BeginTx(context.Background(), sqldriver.TxOptions{})
}

func (c *transactionTestConn) BeginTx(ctx context.Context, _ sqldriver.TxOptions) (sqldriver.Tx, error) {
	state := c.state()
	state.mu.Lock()
	state.beginCount++
	state.beginContext = append(state.beginContext, ctx.Value(transactionTestContextKey{}))
	state.mu.Unlock()
	return &transactionTestTx{scenario: c.scenario, state: state}, nil
}

func (c *transactionTestConn) Ping(context.Context) error { return nil }

func (c *transactionTestConn) ExecContext(ctx context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Result, error) {
	state := c.state()
	state.mu.Lock()
	state.queries = append(state.queries, query)
	state.execContext = append(state.execContext, ctx.Value(transactionTestContextKey{}))
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "SAVEPOINT ") {
		state.savepoints = append(state.savepoints, strings.TrimSpace(query))
	}
	state.mu.Unlock()
	return transactionTestResult{}, nil
}

func (c *transactionTestConn) QueryContext(ctx context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Rows, error) {
	state := c.state()
	state.mu.Lock()
	state.queries = append(state.queries, query)
	state.queryContext = append(state.queryContext, ctx.Value(transactionTestContextKey{}))
	state.mu.Unlock()
	return &transactionTestRows{
		cols: []string{"id", "company_id", "sku", "name", "deleted_at"},
	}, nil
}

func (c *transactionTestConn) CheckNamedValue(*sqldriver.NamedValue) error { return nil }

func (r *transactionTestRows) Columns() []string { return append([]string(nil), r.cols...) }

func (r *transactionTestRows) Close() error { return nil }

func (r *transactionTestRows) Next(dest []sqldriver.Value) error {
	return io.EOF
}

func (r transactionTestResult) LastInsertId() (int64, error) { return 0, nil }

func (r transactionTestResult) RowsAffected() (int64, error) { return 1, nil }

func (t *transactionTestTx) Commit() error {
	state := t.state
	state.mu.Lock()
	defer state.mu.Unlock()
	state.commitCount++
	if state.commitErr != nil {
		return state.commitErr
	}
	return nil
}

func (t *transactionTestTx) Rollback() error {
	state := t.state
	state.mu.Lock()
	defer state.mu.Unlock()
	state.rollbackCount++
	if state.rollbackErr != nil {
		return state.rollbackErr
	}
	return nil
}

func (p *transactionTestTracerProvider) Tracer(string) Tracer {
	return transactionTestTracer{provider: p}
}

func (t transactionTestTracer) Start(ctx context.Context, name string, _ ...SpanOption) (context.Context, Span) {
	t.provider.mu.Lock()
	t.provider.spans = append(t.provider.spans, transactionTestSpanRecord{Name: name})
	idx := len(t.provider.spans) - 1
	t.provider.mu.Unlock()
	return ctx, transactionTestSpan{provider: t.provider, index: idx}
}

func (s transactionTestSpan) End() {}

func (s transactionTestSpan) RecordError(error) {
	s.provider.mu.Lock()
	if s.index >= 0 && s.index < len(s.provider.spans) {
		s.provider.spans[s.index].Errored = true
	}
	s.provider.mu.Unlock()
}

func (s transactionTestSpan) SetAttributes(...Attribute) {}

func transactionTestStateFor(scenario string) *transactionTestState {
	transactionTestStateMu.Lock()
	defer transactionTestStateMu.Unlock()
	if state, ok := transactionTestStates[scenario]; ok {
		return state
	}
	state := &transactionTestState{}
	transactionTestStates[scenario] = state
	return state
}

var (
	transactionTestStateMu sync.Mutex
	transactionTestStates  = map[string]*transactionTestState{}
)

func transactionTestDB(t *testing.T, scenario string, obs ObservabilityConfig) (*DB, *transactionTestState) {
	t.Helper()
	dbConn, err := sql.Open(transactionTestDriverName, scenario)
	if err != nil {
		t.Fatalf("open transaction test db: %v", err)
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
					Name:       "products",
					GoTypeName: "txTestProduct",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "company_id", Scope: schema.ScopeCompany},
						{Name: "sku"},
						{Name: "name"},
						{Name: "deleted_at", SoftDelete: true},
					},
				},
			},
		},
		Observability: obs,
	})
	return db, transactionTestStateFor(scenario)
}

func transactionTestResetState(scenario string) {
	transactionTestStateMu.Lock()
	defer transactionTestStateMu.Unlock()
	transactionTestStates[scenario] = &transactionTestState{}
}

func transactionTestLastQuery(state *transactionTestState) string {
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.queries) == 0 {
		return ""
	}
	return state.queries[len(state.queries)-1]
}

func transactionTestCloneStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

func TestDBTransactionCommitAndRollback(t *testing.T) {
	scenario := t.Name()
	transactionTestResetState(scenario)
	db, state := transactionTestDB(t, scenario, ObservabilityConfig{})

	baseCtx := access.WithContext(context.Background(), access.Context{
		CompanyID: "company-1",
		UserID:    "user-1",
	})
	ctx := context.WithValue(baseCtx, transactionTestContextKey{}, "request-commit")
	err := db.Transaction(ctx, func(tx *DB) error {
		var rows []txTestProduct
		return tx.Find(&rows, Where("sku = ?", "SKU-001"))
	})
	if err != nil {
		t.Fatalf("transaction commit: %v", err)
	}
	if state.beginCount != 1 || state.commitCount != 1 || state.rollbackCount != 0 {
		t.Fatalf("unexpected transaction counts: %+v", state)
	}
	if got := transactionTestLastQuery(state); got == "" || !strings.Contains(strings.ToLower(got), "sku = $1") {
		t.Fatalf("expected query to run inside transaction, got %q", got)
	}
	if len(state.beginContext) != 1 || state.beginContext[0] != "request-commit" {
		t.Fatalf("expected context propagation to begin, got %#v", state.beginContext)
	}
	if len(state.queryContext) != 1 || state.queryContext[0] != "request-commit" {
		t.Fatalf("expected context propagation to query, got %#v", state.queryContext)
	}

	transactionTestResetState(scenario + "-rollback")
	db, state = transactionTestDB(t, scenario+"-rollback", ObservabilityConfig{})
	rollbackErr := errors.New("force rollback")
	err = db.Transaction(ctx, func(tx *DB) error {
		var rows []txTestProduct
		if err := tx.Find(&rows, Where("sku = ?", "SKU-002")); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected rollback callback error, got %v", err)
	}
	if state.beginCount != 1 || state.commitCount != 0 || state.rollbackCount != 1 {
		t.Fatalf("unexpected rollback counts: %+v", state)
	}
}

func TestDBBeginNestedTransactionsUseSavepoints(t *testing.T) {
	scenario := t.Name()
	transactionTestResetState(scenario)
	db, state := transactionTestDB(t, scenario, ObservabilityConfig{})

	ctx := context.WithValue(context.Background(), transactionTestContextKey{}, "request-nested")
	err := db.Transaction(ctx, func(tx *DB) error {
		inner, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		if err := inner.Commit(); err != nil {
			return err
		}
		nested, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		if err := nested.Rollback(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("nested transaction: %v", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.beginCount != 1 {
		t.Fatalf("expected root begin only, got %d", state.beginCount)
	}
	if state.commitCount != 1 {
		t.Fatalf("expected one root commit, got %d", state.commitCount)
	}
	if state.rollbackCount != 0 {
		t.Fatalf("expected no root rollback, got %d", state.rollbackCount)
	}
	var sawSavepoint, sawRelease, sawRollbackTo bool
	for _, query := range state.queries {
		upper := strings.ToUpper(strings.TrimSpace(query))
		if strings.HasPrefix(upper, "SAVEPOINT ") {
			sawSavepoint = true
		}
		if strings.HasPrefix(upper, "RELEASE SAVEPOINT ") {
			sawRelease = true
		}
		if strings.HasPrefix(upper, "ROLLBACK TO SAVEPOINT ") {
			sawRollbackTo = true
		}
	}
	if !sawSavepoint || !sawRelease || !sawRollbackTo {
		t.Fatalf("expected savepoint lifecycle queries, got %#v", state.queries)
	}
}

func TestDBTransactionPropagatesAccessPolicy(t *testing.T) {
	scenario := t.Name()
	transactionTestResetState(scenario)
	db, state := transactionTestDB(t, scenario, ObservabilityConfig{})

	baseCtx := access.WithContext(context.Background(), access.Context{
		CompanyID: "company-1",
		UserID:    "user-1",
	})

	err := db.Transaction(baseCtx, func(tx *DB) error {
		var rows []txTestProduct
		return tx.Find(&rows, Where("sku = ?", "SKU-001"))
	})
	if err != nil {
		t.Fatalf("default policy transaction: %v", err)
	}
	if got := strings.ToLower(transactionTestLastQuery(state)); !strings.Contains(got, "company_id = $2") {
		t.Fatalf("expected company filter in query, got %q", got)
	}
	if got := strings.ToLower(transactionTestLastQuery(state)); !strings.Contains(got, "deleted_at is null") {
		t.Fatalf("expected soft delete filter in query, got %q", got)
	}

	transactionTestResetState(scenario + "-ignore-company")
	db, state = transactionTestDB(t, scenario+"-ignore-company", ObservabilityConfig{})
	policyCtx := access.WithPolicy(baseCtx, access.IgnoreCompany())
	err = db.Transaction(policyCtx, func(tx *DB) error {
		var rows []txTestProduct
		return tx.Find(&rows, Where("sku = ?", "SKU-001"))
	})
	if err != nil {
		t.Fatalf("ignore company transaction: %v", err)
	}
	if got := strings.ToLower(transactionTestLastQuery(state)); strings.Contains(got, "company_id = $2") {
		t.Fatalf("did not expect company filter in query, got %q", got)
	}
	if got := strings.ToLower(transactionTestLastQuery(state)); !strings.Contains(got, "deleted_at is null") {
		t.Fatalf("expected soft delete filter to remain, got %q", got)
	}

	transactionTestResetState(scenario + "-system")
	db, state = transactionTestDB(t, scenario+"-system", ObservabilityConfig{})
	systemCtx := access.WithPolicy(baseCtx, access.System())
	err = db.Transaction(systemCtx, func(tx *DB) error {
		var rows []txTestProduct
		return tx.Find(&rows, Where("sku = ?", "SKU-001"))
	})
	if err != nil {
		t.Fatalf("system transaction: %v", err)
	}
	if got := strings.ToLower(transactionTestLastQuery(state)); strings.Contains(got, "company_id = $2") || strings.Contains(got, "deleted_at is null") {
		t.Fatalf("did not expect row-level policy filters in system mode, got %q", got)
	}
}

func TestDBTransactionObservabilityAndTypedErrors(t *testing.T) {
	scenario := t.Name()
	transactionTestResetState(scenario)
	provider := &transactionTestTracerProvider{}
	db, state := transactionTestDB(t, scenario, ObservabilityConfig{
		Tracing:        true,
		TracerProvider: provider,
	})

	if err := db.Transaction(context.Background(), func(tx *DB) error { return nil }); err != nil {
		t.Fatalf("transaction commit: %v", err)
	}
	if err := db.Transaction(context.Background(), func(tx *DB) error {
		return errors.New("rollback me")
	}); err == nil {
		t.Fatal("expected rollback error")
	}

	provider.mu.Lock()
	names := make([]string, 0, len(provider.spans))
	errored := 0
	for _, span := range provider.spans {
		names = append(names, span.Name)
		if span.Errored {
			errored++
		}
	}
	provider.mu.Unlock()

	expected := map[string]bool{
		"db.transaction": true,
		"db.begin":       true,
		"db.commit":      true,
		"db.rollback":    true,
	}
	for _, name := range names {
		delete(expected, name)
	}
	if len(expected) != 0 {
		t.Fatalf("expected transaction spans, got %v", names)
	}
	if errored == 0 {
		t.Fatalf("expected at least one errored span, got %v", names)
	}

	transactionTestStateMu.Lock()
	transactionTestStates[scenario+"-commit-fail"] = &transactionTestState{commitErr: fmt.Errorf("commit failed")}
	transactionTestStates[scenario+"-rollback-fail"] = &transactionTestState{rollbackErr: fmt.Errorf("rollback failed")}
	transactionTestStateMu.Unlock()

	db, _ = transactionTestDB(t, scenario+"-commit-fail", ObservabilityConfig{})
	err := db.Transaction(context.Background(), func(tx *DB) error { return nil })
	if err == nil {
		t.Fatal("expected commit failure")
	}
	if !errors.Is(err, dormerrors.ErrCommitFailed) {
		t.Fatalf("expected commit failed sentinel, got %T %v", err, err)
	}
	var txErr *dormerrors.TransactionError
	if !errors.As(err, &txErr) {
		t.Fatalf("expected typed transaction error, got %T %v", err, err)
	}
	if txErr == nil || txErr.Operation != "commit" {
		t.Fatalf("unexpected transaction error: %#v", txErr)
	}

	db, state = transactionTestDB(t, scenario+"-rollback-fail", ObservabilityConfig{})
	callbackErr := errors.New("callback failure")
	err = db.Transaction(context.Background(), func(tx *DB) error { return callbackErr })
	if err == nil {
		t.Fatal("expected rollback failure")
	}
	if !errors.Is(err, callbackErr) {
		t.Fatalf("expected callback error to be preserved, got %v", err)
	}
	if !errors.Is(err, dormerrors.ErrRollbackFailed) {
		t.Fatalf("expected rollback failed sentinel, got %T %v", err, err)
	}
	if !errors.As(err, &txErr) {
		t.Fatalf("expected typed rollback error, got %T %v", err, err)
	}
	if txErr == nil || txErr.Operation != "rollback" {
		t.Fatalf("unexpected rollback error: %#v", txErr)
	}

	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatal("expected closed transaction error")
	} else if !errors.Is(err, dormerrors.ErrTransactionClosed) {
		t.Fatalf("expected transaction closed sentinel, got %T %v", err, err)
	}
	if state == nil {
		t.Fatal("expected transaction state")
	}
}

type txTestProduct struct {
	ID        string
	CompanyID string
	SKU       string
	Name      string
}
