package orm

import (
	"database/sql"
	"testing"
	"time"
)

func TestDefaultObservabilityConfig(t *testing.T) {
	cfg := DefaultObservabilityConfig()
	if cfg.TraceSQL != TraceSQLMetadata {
		t.Fatalf("expected metadata trace mode, got %q", cfg.TraceSQL)
	}
	if cfg.SQLLogging != SQLLogDisabled {
		t.Fatalf("expected disabled logging, got %q", cfg.SQLLogging)
	}
	if cfg.Enabled() {
		t.Fatalf("expected disabled config")
	}
}

func TestObservabilityConfigValidate(t *testing.T) {
	cfg := ObservabilityConfig{TraceSQL: TraceSQLStatement, SQLLogging: SQLLogTrace, SlowQueryThreshold: time.Second}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
	if err := (ObservabilityConfig{TraceSQL: TraceSQLMode("bad")}).Validate(); err == nil {
		t.Fatalf("expected invalid trace mode error")
	}
	if err := (ObservabilityConfig{SQLLogging: SQLLogMode("bad")}).Validate(); err == nil {
		t.Fatalf("expected invalid mode error")
	}
	if err := (ObservabilityConfig{SlowQueryThreshold: -time.Second}).Validate(); err == nil {
		t.Fatalf("expected negative threshold error")
	}
}

func TestObservabilityConfigMasking(t *testing.T) {
	cfg := ObservabilityConfig{MaskedFields: []string{"custom_secret"}}.Normalized()
	if !cfg.ShouldMask("password") {
		t.Fatalf("expected password to be masked")
	}
	if !cfg.ShouldMask("custom_secret") {
		t.Fatalf("expected custom_secret to be masked")
	}
	if cfg.ShouldMask("harmless") {
		t.Fatalf("did not expect harmless to be masked")
	}
}

func TestNewNormalizesObservabilityConfig(t *testing.T) {
	db := New(Config{
		Observability: ObservabilityConfig{
			MaskedFields: []string{"custom_secret"},
		},
	})
	if db == nil {
		t.Fatalf("expected db")
	}
	if !db.observability.ShouldMask("password") {
		t.Fatalf("expected default mask list to be present")
	}
	if !db.observability.ShouldMask("custom_secret") {
		t.Fatalf("expected custom mask to be present")
	}
}

func TestSQLTraceVisibilityAttributes(t *testing.T) {
	db := New(Config{
		DriverName: "postgres",
		Dialect:    nil,
		Observability: ObservabilityConfig{
			TraceSQL: TraceSQLStatementWithArgs,
		},
	})
	db.dialect = nil
	entry := SQLLogEntry{
		SQL:          "select * from users where password = $1",
		Args:         []any{sql.Named("password", "secret")},
		Visibility:   TraceSQLStatementWithArgs,
		Operation:    "query",
		Table:        "users",
		Duration:     time.Second,
		AffectedRows: 3,
		Err:          nil,
	}
	attrs := sqlTraceVisibilityAttrs(entry, db)
	foundStatement := false
	foundArgs := false
	for _, attr := range attrs {
		if attr.Key == "db.statement" {
			foundStatement = true
		}
		if attr.Key == "db.statement.args" {
			foundArgs = true
		}
	}
	if !foundStatement || !foundArgs {
		t.Fatalf("expected statement and args attrs, got %#v", attrs)
	}
}
