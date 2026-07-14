package seed

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
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
	})

	type Role struct {
		ID        int
		Code      string
		Name      string
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	if err := Sync(context.Background(), db, &Role{Code: "ADMIN", Name: "Administrator"}, Key("Code")); err != nil {
		t.Fatal(err)
	}
	query := strings.ToLower(seedTestLastQuery())
	if !strings.Contains(query, `insert into "roles"`) {
		t.Fatalf("expected insert query, got %q", query)
	}
}

func TestSyncUpdatesExistingRecord(t *testing.T) {
	seedTestReset([][]driver.Value{
		{int64(1), "ADMIN", "Old", time.Now().UTC(), time.Now().UTC()},
	}[0])
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
	})

	type Role struct {
		ID        int
		Code      string
		Name      string
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	if err := Sync(context.Background(), db, &Role{Code: "ADMIN", Name: "Administrator"}, Key("Code")); err != nil {
		t.Fatal(err)
	}
	query := strings.ToLower(seedTestLastQuery())
	if !strings.Contains(query, `update "roles" set`) {
		t.Fatalf("expected update query, got %q", query)
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
