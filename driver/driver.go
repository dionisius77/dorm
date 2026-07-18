package driver

import (
	"context"
	"database/sql"

	"github.com/dionisius77/dorm/dialect"
	"github.com/dionisius77/dorm/schema"
)

// PreflightConfig describes an optional schema preflight check.
type PreflightConfig struct {
	Enabled      bool
	Root         string
	SnapshotPath string
	SchemaName   string
}

// ConnectionInfo describes connection metadata that can be exposed for traces.
type ConnectionInfo struct {
	System        string
	Name          string
	Namespace     string
	ServerAddress string
	ServerPort    int
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

// InspectorProvider exposes the schema inspector associated with a driver.
type InspectorProvider interface {
	Inspector() schema.Inspector
}

// ConnectionInfoProvider exposes semantic connection metadata for observability.
type ConnectionInfoProvider interface {
	ConnectionInfo() ConnectionInfo
}
