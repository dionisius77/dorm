package driver

import (
	"context"
	"database/sql"

	"github.com/dionisius77/dorm/dialect"
)

// PreflightConfig describes an optional schema preflight check.
type PreflightConfig struct {
	Enabled      bool
	Root         string
	SnapshotPath string
	SchemaName   string
}

// Driver provides database-specific connection and dialect behavior.
type Driver interface {
	Validate() error
	Name() string
	Dialect() dialect.Dialect
	Open(context.Context) (*sql.DB, error)
}

// PreflightProvider exposes optional schema preflight settings for Open().
type PreflightProvider interface {
	PreflightConfig() PreflightConfig
}
