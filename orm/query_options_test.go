package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dionisius77/dorm/access"
	postgresdialect "github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/schema"
)

const queryOptionTestDriverName = "orm-query-options"

var queryOptionTestDriverOnce sync.Once

type queryOptionTestState struct {
	mu        sync.Mutex
	lastQuery string
	lastArgs  []any
	queries   []string
}

type queryOptionTestDriver struct{}

type queryOptionTestConn struct {
	scenario string
}

type queryOptionTestRows struct {
	cols []string
	data [][]driver.Value
	idx  int
}

type queryOptionTestResult struct{}

type queryOptionUser struct {
	ID        string
	Email     string
	CompanyID string
	Status    string
	CreatedAt time.Time
}

func init() {
	queryOptionTestDriverOnce.Do(func() {
		sql.Register(queryOptionTestDriverName, queryOptionTestDriver{})
	})
}

func (queryOptionTestDriver) Open(name string) (driver.Conn, error) {
	return &queryOptionTestConn{scenario: name}, nil
}

func (c *queryOptionTestConn) Close() error { return nil }

func (c *queryOptionTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions not supported")
}

func (c *queryOptionTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}

func (c *queryOptionTestConn) Ping(context.Context) error { return nil }

func (c *queryOptionTestConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return queryOptionTestResult{}, nil
}

func (c *queryOptionTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	state := queryOptionTestStateFor(c.scenario)
	state.mu.Lock()
	state.lastQuery = query
	state.lastArgs = namedValuesToAny(args)
	state.queries = append(state.queries, query)
	state.mu.Unlock()

	upper := strings.ToUpper(strings.TrimSpace(query))
	switch {
	case strings.HasPrefix(upper, "SELECT COUNT(*) FROM ("):
		return &queryOptionTestRows{
			cols: []string{"count"},
			data: [][]driver.Value{{int64(1)}},
		}, nil
	case strings.HasPrefix(upper, "SELECT 1 FROM ("):
		return &queryOptionTestRows{
			cols: []string{"exists"},
			data: [][]driver.Value{{int64(1)}},
		}, nil
	default:
		return &queryOptionTestRows{
			cols: []string{"id", "email", "company_id", "status", "created_at"},
			data: [][]driver.Value{{
				"user-1",
				"alice@example.com",
				"company-1",
				"active",
				time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC),
			}},
		}, nil
	}
}

func (c *queryOptionTestConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (queryOptionTestResult) LastInsertId() (int64, error) { return 0, nil }

func (queryOptionTestResult) RowsAffected() (int64, error) { return 1, nil }

func (r *queryOptionTestRows) Columns() []string { return append([]string(nil), r.cols...) }

func (r *queryOptionTestRows) Close() error { return nil }

func (r *queryOptionTestRows) Next(dest []driver.Value) error {
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

func queryOptionTestStateFor(scenario string) *queryOptionTestState {
	queryOptionTestStateMu.Lock()
	defer queryOptionTestStateMu.Unlock()
	if state, ok := queryOptionTestStates[scenario]; ok {
		return state
	}
	state := &queryOptionTestState{}
	queryOptionTestStates[scenario] = state
	return state
}

func queryOptionTestDB(t *testing.T, tracer TracerProvider) (*DB, *queryOptionTestState) {
	t.Helper()
	scenario := t.Name()
	sqlDB, err := sql.Open(queryOptionTestDriverName, scenario)
	if err != nil {
		t.Fatal(err)
	}
	state := queryOptionTestStateFor(scenario)
	state.mu.Lock()
	state.lastQuery = ""
	state.lastArgs = nil
	state.queries = nil
	state.mu.Unlock()
	table := &schema.Table{
		Name:       "users",
		GoTypeName: "queryOptionUser",
		Columns: []*schema.Column{
			{Name: "id", Type: schema.Type{Name: "text", Kind: schema.TypeString}},
			{Name: "email", Type: schema.Type{Name: "text", Kind: schema.TypeString}},
			{Name: "company_id", Scope: schema.ScopeCompany},
			{Name: "status", Type: schema.Type{Name: "text", Kind: schema.TypeString}},
			{Name: "created_at", Type: schema.Type{Name: "timestamptz", Kind: schema.TypeTime}},
		},
	}
	db := New(Config{
		DB:      sqlDB,
		Dialect: postgresdialect.New(),
		Schema: &schema.Schema{
			Name:   "public",
			Tables: []*schema.Table{table},
		},
		Observability: ObservabilityConfig{
			Tracing:        tracer != nil,
			TracerProvider: tracer,
		},
	})
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db, state
}

func TestQueryOptionsComposeAcrossClauseFamilies(t *testing.T) {
	provider := &rawTestTracerProvider{}
	db, state := queryOptionTestDB(t, provider)
	ctx := access.WithContext(context.Background(), access.Context{CompanyID: "company-1"})

	var users []queryOptionUser
	err := db.WithContext(ctx).Find(&users,
		Offset(40),
		Select("users.id, users.email, users.company_id, users.status, users.created_at"),
		Distinct(),
		LeftJoin("roles r", "r.id = users.role_id"),
		Where("users.status = ?", "active"),
		GroupBy("users.company_id"),
		Having("COUNT(*) > ?", 5),
		OrderBy("users.created_at DESC"),
		Limit(20),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected one row, got %d", len(users))
	}

	state.mu.Lock()
	got := state.lastQuery
	args := append([]any(nil), state.lastArgs...)
	state.mu.Unlock()

	want := `SELECT DISTINCT users.id, users.email, users.company_id, users.status, users.created_at FROM "users" LEFT JOIN roles r ON r.id = users.role_id WHERE users.status = $1 AND company_id = $2 GROUP BY users.company_id HAVING COUNT(*) > $3 ORDER BY users.created_at DESC LIMIT 20 OFFSET 40`
	if got != want {
		t.Fatalf("unexpected sql:\nwant %s\ngot  %s", want, got)
	}
	if len(args) != 3 || args[0] != "active" || args[1] != "company-1" || args[2] != 5 {
		t.Fatalf("unexpected args: %#v", args)
	}

	if len(provider.spans) == 0 {
		t.Fatal("expected query span")
	}
	attrs := provider.spans[0].Attributes
	if attrs["orm.operation"] != "find" {
		t.Fatalf("expected find operation attr, got %#v", attrs)
	}
	if attrs["orm.query.join_count"] != 1 || attrs["orm.query.distinct"] != true {
		t.Fatalf("expected query metadata attrs, got %#v", attrs)
	}
}

func TestQueryOptionsRenderJoinVariants(t *testing.T) {
	db, state := queryOptionTestDB(t, nil)
	ctx := access.WithContext(context.Background(), access.Context{CompanyID: "company-1"})

	var users []queryOptionUser
	if err := db.WithContext(ctx).Find(&users,
		Select("users.id, users.email, users.company_id, users.status, users.created_at"),
		RightJoin("teams t", "t.user_id = users.id"),
		InnerJoin("permissions p", "p.user_id = users.id"),
		CrossJoin("tenants ten"),
		Where("users.status = ?", "active"),
	); err != nil {
		t.Fatal(err)
	}

	state.mu.Lock()
	got := state.lastQuery
	state.mu.Unlock()
	for _, want := range []string{"RIGHT JOIN teams t", "INNER JOIN permissions p", "CROSS JOIN tenants ten"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %s", want, got)
		}
	}
	if !strings.Contains(got, "WHERE users.status = $1 AND company_id = $2") {
		t.Fatalf("expected policy and user where clauses, got %s", got)
	}
}

func TestQueryOptionsOrderIsDeterministic(t *testing.T) {
	db, state := queryOptionTestDB(t, nil)
	ctx := access.WithContext(context.Background(), access.Context{CompanyID: "company-1"})

	var users []queryOptionUser
	optsA := []QueryOption{
		Limit(5),
		Select("users.id, users.email, users.company_id, users.status, users.created_at"),
		Where("users.status = ?", "active"),
		OrderBy("users.created_at DESC"),
		Offset(10),
	}
	if err := db.WithContext(ctx).Find(&users, optsA...); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	first := state.lastQuery
	state.mu.Unlock()

	optsB := []QueryOption{
		Offset(10),
		OrderBy("users.created_at DESC"),
		Where("users.status = ?", "active"),
		Select("users.id, users.email, users.company_id, users.status, users.created_at"),
		Limit(5),
	}
	if err := db.WithContext(ctx).Find(&users, optsB...); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	second := state.lastQuery
	state.mu.Unlock()

	if first != second {
		t.Fatalf("expected deterministic sql, got\n1: %s\n2: %s", first, second)
	}
}

func TestQueryOptionsCountAndExists(t *testing.T) {
	db, state := queryOptionTestDB(t, nil)
	ctx := access.WithContext(context.Background(), access.Context{CompanyID: "company-1"})
	opts := []QueryOption{
		Select("users.id, users.email, users.company_id, users.status, users.created_at"),
		Where("users.status = ?", "active"),
		Limit(3),
	}

	count, err := db.WithContext(ctx).Count(&queryOptionUser{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
	state.mu.Lock()
	countSQL := state.lastQuery
	state.mu.Unlock()
	if !strings.Contains(countSQL, "SELECT COUNT(*) FROM (") {
		t.Fatalf("expected wrapped count sql, got %s", countSQL)
	}

	exists, err := db.WithContext(ctx).Exists(&queryOptionUser{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected exists to be true")
	}
	state.mu.Lock()
	existsSQL := state.lastQuery
	state.mu.Unlock()
	if !strings.Contains(existsSQL, "SELECT 1 FROM (") {
		t.Fatalf("expected wrapped exists sql, got %s", existsSQL)
	}
}

func TestQueryOptionsValidateInputs(t *testing.T) {
	db, _ := queryOptionTestDB(t, nil)
	ctx := context.Background()
	var users []queryOptionUser

	tests := []struct {
		name string
		opt  QueryOption
	}{
		{name: "select", opt: Select("")},
		{name: "limit", opt: Limit(-1)},
		{name: "join", opt: LeftJoin("", "id = users.id")},
		{name: "group", opt: GroupBy("")},
		{name: "having", opt: Having("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.WithContext(ctx).Find(&users, tt.opt)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !errkind.Is(err, errkind.KindConfiguration) {
				t.Fatalf("expected configuration error, got %T %v", err, err)
			}
		})
	}
}

var (
	queryOptionTestStateMu sync.Mutex
	queryOptionTestStates  = map[string]*queryOptionTestState{}
)
