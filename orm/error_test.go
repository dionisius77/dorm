package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"testing"

	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/schema"
)

var registerORMErrorDriverOnce sync.Once

type failingQueryDriver struct{}

type failingQueryConn struct{}

type failingQueryRows struct{}

func init() {
	registerORMErrorDriverOnce.Do(func() {
		sql.Register("orm-error-query", failingQueryDriver{})
	})
}

func (failingQueryDriver) Open(name string) (driver.Conn, error) {
	_ = name
	return failingQueryConn{}, nil
}

func (failingQueryConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (failingQueryConn) Close() error { return nil }

func (failingQueryConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("tx not supported") }

func (failingQueryConn) Ping(context.Context) error { return nil }

func (failingQueryConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

func (failingQueryConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, fmt.Errorf("boom")
}

func (failingQueryConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (failingQueryRows) Columns() []string { return []string{"id"} }

func (failingQueryRows) Close() error { return nil }

func (failingQueryRows) Next(dest []driver.Value) error { return io.EOF }

func TestCreateReturnsRuntimeQueryError(t *testing.T) {
	db, err := sql.Open("orm-error-query", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	session := New(Config{
		DB:      db,
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "users",
					GoTypeName: "User",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "email"},
					},
				},
			},
		},
	}).WithContext(context.Background())

	type User struct {
		ID    int
		Email string
	}

	err = session.Create(&User{Email: "alice@example.com"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errkind.ErrRuntimeQuery) {
		t.Fatalf("expected runtime query error, got %T %v", err, err)
	}
}

func TestReflectTypeToSchemaTypeUsesRegisteredCustomType(t *testing.T) {
	type Currency string
	schema.RegisterCustomType("currency", schema.Type{Name: "numeric", Kind: schema.TypeFloat})
	if got := reflectTypeToSchemaType(reflect.TypeOf(Currency(""))); got.Name != "numeric" {
		t.Fatalf("expected registered type mapping, got %#v", got)
	}
}

func TestAssignReflectValueUsesScanner(t *testing.T) {
	type Money struct {
		Amount currencyAmount
	}
	rv := reflect.ValueOf(&Money{}).Elem().FieldByName("Amount")
	assignReflectValue(rv, "12.34")
	if got := rv.Interface().(currencyAmount).Value; got != "12.34" {
		t.Fatalf("expected scanner to populate custom value, got %q", got)
	}
}

type currencyAmount struct {
	Value string
}

func (c *currencyAmount) Scan(src any) error {
	c.Value = fmt.Sprint(src)
	return nil
}
