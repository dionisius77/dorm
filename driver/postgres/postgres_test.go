package postgres

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

const testSQLDriverName = "dorm-postgres-driver-test"

var testSQLDriverOnce sync.Once

type testSQLDriver struct{}

type testSQLConn struct{}

type testSQLResult struct{}

func init() {
	testSQLDriverOnce.Do(func() {
		sql.Register(testSQLDriverName, testSQLDriver{})
	})
}

func (testSQLDriver) Open(string) (sqldriver.Conn, error) {
	return testSQLConn{}, nil
}

func (testSQLConn) Prepare(string) (sqldriver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}

func (testSQLConn) Close() error { return nil }

func (testSQLConn) Begin() (sqldriver.Tx, error) {
	return nil, errors.New("transactions not supported")
}

func (testSQLConn) Ping(context.Context) error { return nil }

func (testSQLConn) ExecContext(context.Context, string, []sqldriver.NamedValue) (sqldriver.Result, error) {
	return testSQLResult{}, nil
}

func (testSQLConn) QueryContext(context.Context, string, []sqldriver.NamedValue) (sqldriver.Rows, error) {
	return &testSQLRows{}, nil
}

func (testSQLConn) CheckNamedValue(*sqldriver.NamedValue) error { return nil }

func (testSQLResult) LastInsertId() (int64, error) { return 0, nil }

func (testSQLResult) RowsAffected() (int64, error) { return 0, nil }

type testSQLRows struct{}

func (r *testSQLRows) Columns() []string { return []string{"value"} }

func (r *testSQLRows) Close() error { return nil }

func (r *testSQLRows) Next(dest []sqldriver.Value) error {
	return io.EOF
}

func TestDriverOpenAppliesPoolConfiguration(t *testing.T) {
	drv := New(Config{
		DSN:             "pool-test",
		DriverName:      testSQLDriverName,
		MaxOpenConns:    17,
		MaxIdleConns:    11,
		ConnMaxLifetime: 3 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	})
	db, err := drv.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := db.Stats().MaxOpenConnections; got != 17 {
		t.Fatalf("expected max open connections 17, got %d", got)
	}
}

func TestDriverOpenHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	drv := New(Config{
		DSN:        "context-test",
		DriverName: testSQLDriverName,
	})
	_, err := drv.Open(ctx)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation to wrap context.Canceled, got %v", err)
	}
}
