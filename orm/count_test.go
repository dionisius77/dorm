package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/schema"
)

var registerCountTestDriverOnce sync.Once

type countTestDriver struct{}

type countTestConn struct{}

type countTestRows struct {
	cols []string
	data [][]driver.Value
	idx  int
}

func init() {
	registerCountTestDriverOnce.Do(func() {
		sql.Register("orm-count", countTestDriver{})
	})
}

func (countTestDriver) Open(name string) (driver.Conn, error) {
	_ = name
	return countTestConn{}, nil
}

func (countTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (countTestConn) Close() error { return nil }

func (countTestConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("tx not supported") }

func (countTestConn) Ping(context.Context) error { return nil }

func (countTestConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

func (countTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if !strings.Contains(normalized, "count(*)") {
		return nil, fmt.Errorf("expected count query, got %s", query)
	}
	if !strings.Contains(normalized, `from "users"`) {
		return nil, fmt.Errorf("expected users table, got %s", query)
	}
	return &countTestRows{
		cols: []string{"count"},
		data: [][]driver.Value{{int64(7)}},
	}, nil
}

func (countTestConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (r *countTestRows) Columns() []string { return append([]string(nil), r.cols...) }

func (r *countTestRows) Close() error { return nil }

func (r *countTestRows) Next(dest []driver.Value) error {
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

func TestSessionCountReturnsRowCount(t *testing.T) {
	dbConn, err := sql.Open("orm-count", "")
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
						{Name: "email"},
						{Name: "deleted_at", DeletedAt: true, SoftDelete: true},
					},
				},
			},
		},
	}).WithContext(context.Background())

	type User struct {
		ID        int
		Email     string
		DeletedAt *string
	}

	count, err := session.Count(&User{}, Where("email = ?", "alice@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if count != 7 {
		t.Fatalf("expected count 7, got %d", count)
	}
}
