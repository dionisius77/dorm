package driver

import (
	"context"
	"database/sql"

	"github.com/dionisius77/dorm/dialect"
)

// Driver provides database-specific connection and dialect behavior.
type Driver interface {
	Validate() error
	Name() string
	Dialect() dialect.Dialect
	Open(context.Context) (*sql.DB, error)
}
