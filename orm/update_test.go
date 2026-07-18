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
	"github.com/dionisius77/dorm/errkind"
	dormerrors "github.com/dionisius77/dorm/errors"
	"github.com/dionisius77/dorm/model"
	"github.com/dionisius77/dorm/schema"
)

var registerUpdateTestDriverOnce sync.Once
var registerUpdateConflictTestDriverOnce sync.Once

type updateTestDriver struct{}
type updateConflictTestDriver struct{}

type updateTestConn struct{}
type updateConflictTestConn struct{}

type updateTestRows struct {
	cols []string
	data [][]sqldriver.Value
	idx  int
}

type updateTestResult struct{}

func init() {
	registerUpdateTestDriverOnce.Do(func() {
		sql.Register("orm-update", updateTestDriver{})
	})
	registerUpdateConflictTestDriverOnce.Do(func() {
		sql.Register("orm-update-conflict", updateConflictTestDriver{})
	})
}

func (updateTestDriver) Open(name string) (sqldriver.Conn, error) {
	_ = name
	return updateTestConn{}, nil
}

func (updateConflictTestDriver) Open(name string) (sqldriver.Conn, error) {
	_ = name
	return updateConflictTestConn{}, nil
}

func (updateTestConn) Prepare(string) (sqldriver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (updateTestConn) Close() error { return nil }

func (updateTestConn) Begin() (sqldriver.Tx, error) { return nil, fmt.Errorf("tx not supported") }

func (updateTestConn) Ping(context.Context) error { return nil }

func (updateTestConn) ExecContext(_ context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Result, error) {
	updateTestStore(query)
	return updateTestResult{}, nil
}

func (updateTestConn) QueryContext(_ context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Rows, error) {
	updateTestStore(query)
	normalized := strings.ToLower(strings.TrimSpace(query))
	if strings.Contains(normalized, "returning") {
		return &updateTestRows{
			cols: []string{"id", "version"},
			data: [][]sqldriver.Value{{int64(7), int64(2)}},
		}, nil
	}
	return &updateTestRows{cols: []string{"ok"}}, nil
}

func (updateConflictTestConn) Prepare(string) (sqldriver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (updateConflictTestConn) Close() error { return nil }

func (updateConflictTestConn) Begin() (sqldriver.Tx, error) {
	return nil, fmt.Errorf("tx not supported")
}

func (updateConflictTestConn) Ping(context.Context) error { return nil }

func (updateConflictTestConn) ExecContext(_ context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Result, error) {
	updateTestStore(query)
	return updateTestResult{}, nil
}

func (updateConflictTestConn) QueryContext(_ context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Rows, error) {
	updateTestStore(query)
	normalized := strings.ToLower(strings.TrimSpace(query))
	if strings.Contains(normalized, "returning") {
		return &updateTestRows{cols: []string{"id", "version"}}, nil
	}
	return &updateTestRows{cols: []string{"ok"}}, nil
}

func (updateConflictTestConn) CheckNamedValue(*sqldriver.NamedValue) error { return nil }

func (updateTestConn) CheckNamedValue(*sqldriver.NamedValue) error { return nil }

func (updateTestResult) LastInsertId() (int64, error) { return 0, nil }

func (updateTestResult) RowsAffected() (int64, error) { return 1, nil }

func (r *updateTestRows) Columns() []string { return append([]string(nil), r.cols...) }

func (r *updateTestRows) Close() error { return nil }

func (r *updateTestRows) Next(dest []sqldriver.Value) error {
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

var (
	updateTestMu      sync.Mutex
	updateTestQueries []string
)

func updateTestStore(query string) {
	updateTestMu.Lock()
	defer updateTestMu.Unlock()
	updateTestQueries = append(updateTestQueries, query)
}

func updateTestReset() {
	updateTestMu.Lock()
	defer updateTestMu.Unlock()
	updateTestQueries = nil
}

func updateTestLastQuery() string {
	updateTestMu.Lock()
	defer updateTestMu.Unlock()
	if len(updateTestQueries) == 0 {
		return ""
	}
	return updateTestQueries[len(updateTestQueries)-1]
}

func TestSessionUpdateUsesPrimaryKey(t *testing.T) {
	updateTestReset()
	dbConn, err := sql.Open("orm-update", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	session := New(Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "users",
					GoTypeName: "User",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "name"},
					},
				},
			},
		},
	}).WithContext(context.Background())

	type User struct {
		ID   int
		Name string
	}

	if err := session.Update(&User{ID: 7, Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	query := strings.ToLower(updateTestLastQuery())
	if !strings.Contains(query, `update "users" set`) {
		t.Fatalf("expected update query, got %q", query)
	}
	if !strings.Contains(query, `"id" = $2`) {
		t.Fatalf("expected primary key where clause, got %q", query)
	}
}

func TestSessionUpdateUsesOptimisticLocking(t *testing.T) {
	updateTestReset()
	dbConn, err := sql.Open("orm-update", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	session := New(Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "users",
					GoTypeName: "User",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "version", Version: true},
						{Name: "name"},
					},
				},
			},
		},
	}).WithContext(context.Background())

	type User struct {
		ID int
		model.Version
		Name string
	}

	user := &User{ID: 7, Version: model.Version{Version: 1}, Name: "alice"}
	if err := session.Update(user); err != nil {
		t.Fatal(err)
	}
	query := strings.ToLower(updateTestLastQuery())
	if !strings.Contains(query, `update "users" set`) {
		t.Fatalf("expected update query, got %q", query)
	}
	if !strings.Contains(query, `"version" = "version" + 1`) {
		t.Fatalf("expected optimistic-lock version increment, got %q", query)
	}
	if !strings.Contains(query, `"version" = $3`) {
		t.Fatalf("expected version predicate, got %q", query)
	}
	if user.Version.Version != 2 {
		t.Fatalf("expected model version to be refreshed, got %#v", user.Version)
	}
}

func TestSessionUpdateWhereUsesExplicitFilters(t *testing.T) {
	updateTestReset()
	dbConn, err := sql.Open("orm-update", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	session := New(Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "users",
					GoTypeName: "User",
					Columns: []*schema.Column{
						{Name: "name"},
						{Name: "status"},
					},
				},
			},
		},
	}).WithContext(context.Background())

	type User struct {
		Name   string
		Status string
	}

	if err := session.UpdateWhere(&User{Name: "alice"}, Where("status = ?", "active")); err != nil {
		t.Fatal(err)
	}
	query := strings.ToLower(updateTestLastQuery())
	if !strings.Contains(query, `update "users" set`) {
		t.Fatalf("expected update query, got %q", query)
	}
	if !strings.Contains(query, `where status = $3`) {
		t.Fatalf("expected explicit where clause, got %q", query)
	}
}

func TestSessionUpdateWhereRejectsMissingFilters(t *testing.T) {
	updateTestReset()
	dbConn, err := sql.Open("orm-update", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	session := New(Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "users",
					GoTypeName: "User",
					Columns: []*schema.Column{
						{Name: "name"},
					},
				},
			},
		},
	}).WithContext(context.Background())

	type User struct {
		Name string
	}

	err = session.UpdateWhere(&User{Name: "alice"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errkind.ErrInvalidSchema) {
		t.Fatalf("expected invalid schema error, got %T %v", err, err)
	}
}

func TestSessionUpdateWhereAppliesAccessPredicates(t *testing.T) {
	updateTestReset()
	dbConn, err := sql.Open("orm-update", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	session := New(Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "users",
					GoTypeName: "User",
					Columns: []*schema.Column{
						{Name: "company_id", Scope: schema.ScopeCompany},
						{Name: "version", Version: true},
						{Name: "name"},
					},
				},
			},
		},
	}).WithContext(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-1",
	}))

	type User struct {
		CompanyID string
		model.Version
		Name string
	}

	if err := session.UpdateWhere(&User{Version: model.Version{Version: 4}, Name: "alice"}, Where("name = ?", "bob")); err != nil {
		t.Fatal(err)
	}
	query := strings.ToLower(updateTestLastQuery())
	if !strings.Contains(query, `where name = $2`) {
		t.Fatalf("expected explicit filter, got %q", query)
	}
	if !strings.Contains(query, `company_id = $3`) {
		t.Fatalf("expected access predicate, got %q", query)
	}
	if !strings.Contains(query, `"version" = $4`) {
		t.Fatalf("expected version predicate, got %q", query)
	}
}

func TestSessionUpdateDetectsOptimisticLockConflict(t *testing.T) {
	updateTestReset()
	dbConn, err := sql.Open("orm-update-conflict", "")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	session := New(Config{
		DB:      dbConn,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "users",
					GoTypeName: "User",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "version", Version: true},
						{Name: "name"},
					},
				},
			},
		},
	}).WithContext(context.Background())

	type User struct {
		ID int
		model.Version
		Name string
	}

	err = session.Update(&User{ID: 7, Version: model.Version{Version: 9}, Name: "alice"})
	if err == nil {
		t.Fatal("expected optimistic lock error")
	}
	if !errors.Is(err, dormerrors.ErrOptimisticLockFailed) {
		t.Fatalf("expected optimistic lock error, got %T %v", err, err)
	}
}
