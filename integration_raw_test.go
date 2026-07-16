package dorm_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	dormerrors "github.com/dionisius77/dorm/errors"
	"github.com/dionisius77/dorm/internal/itest"
	"github.com/dionisius77/dorm/orm"
)

type integrationRawTracerProvider struct {
	mu    sync.Mutex
	spans []integrationRawSpanRecord
}

type integrationRawSpanRecord struct {
	Name       string
	Attributes map[string]any
}

type integrationRawTracer struct {
	provider *integrationRawTracerProvider
}

type integrationRawSpan struct {
	provider *integrationRawTracerProvider
	index    int
}

func (p *integrationRawTracerProvider) Tracer(string) orm.Tracer {
	return integrationRawTracer{provider: p}
}

func (t integrationRawTracer) Start(ctx context.Context, name string, _ ...orm.SpanOption) (context.Context, orm.Span) {
	t.provider.mu.Lock()
	t.provider.spans = append(t.provider.spans, integrationRawSpanRecord{
		Name:       name,
		Attributes: map[string]any{},
	})
	idx := len(t.provider.spans) - 1
	t.provider.mu.Unlock()
	return ctx, integrationRawSpan{provider: t.provider, index: idx}
}

func (s integrationRawSpan) End() {}

func (s integrationRawSpan) RecordError(error) {}

func (s integrationRawSpan) SetAttributes(attrs ...orm.Attribute) {
	s.provider.mu.Lock()
	defer s.provider.mu.Unlock()
	if s.index < 0 || s.index >= len(s.provider.spans) {
		return
	}
	record := s.provider.spans[s.index]
	for _, attr := range attrs {
		record.Attributes[attr.Key] = attr.Value
	}
	s.provider.spans[s.index] = record
}

type rawIntegrationUser struct {
	ID    string
	Email string
}

func TestIntegrationRawSQLScanExecAndTransaction(t *testing.T) {
	project := itest.NewProject(t)
	db := project.OpenDB(t, orm.ObservabilityConfig{
		Tracing:        true,
		TraceSQL:       orm.TraceSQLStatement,
		TracerProvider: &integrationRawTracerProvider{},
	}, false)
	sqlDB := project.OpenSQL(t)
	defer sqlDB.Close()

	_, err := sqlDB.ExecContext(context.Background(), `
		CREATE TABLE raw_users (
			id text PRIMARY KEY,
			email text NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create raw_users table: %v", err)
	}
	_, err = sqlDB.ExecContext(context.Background(), `INSERT INTO raw_users (id, email) VALUES ($1, $2)`, "user-1", "alice@example.com")
	if err != nil {
		t.Fatalf("insert raw user: %v", err)
	}

	var users []rawIntegrationUser
	if err := db.Raw(context.Background(), `SELECT id, email FROM raw_users WHERE email = ?`, "alice@example.com").
		WithoutPolicy().
		Scan(&users); err != nil {
		t.Fatalf("raw scan: %v", err)
	}
	if len(users) != 1 || users[0].ID != "user-1" || users[0].Email != "alice@example.com" {
		t.Fatalf("unexpected scan result: %#v", users)
	}

	result, err := db.Raw(context.Background(), `
		UPDATE raw_users
		SET email = ?
		WHERE id = ?
	`, "alice+1@example.com", "user-1").
		WithoutPolicy().
		Exec()
	if err != nil {
		t.Fatalf("raw exec: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("expected one affected row, got %d", rowsAffected)
	}

	if err := db.Transaction(context.Background(), func(tx *orm.DB) error {
		_, err := tx.Raw(context.Background(), `
			UPDATE raw_users
			SET email = ?
			WHERE id = ?
		`, "rollback@example.com", "user-1").
			WithoutPolicy().
			Exec()
		if err != nil {
			return err
		}
		return fmt.Errorf("rollback raw sql")
	}); err == nil {
		t.Fatal("expected transaction rollback error")
	}

	if err := sqlDB.QueryRowContext(context.Background(), `SELECT email FROM raw_users WHERE id = $1`, "user-1").Scan(&users[0].Email); err != nil {
		t.Fatalf("verify transactional rollback: %v", err)
	}
	if users[0].Email != "alice+1@example.com" {
		t.Fatalf("expected rollback to preserve committed email, got %q", users[0].Email)
	}
}

func TestIntegrationRawSQLWithoutPolicyFails(t *testing.T) {
	project := itest.NewProject(t)
	db := project.OpenDB(t, orm.ObservabilityConfig{}, false)
	sqlDB := project.OpenSQL(t)
	defer sqlDB.Close()

	_, err := sqlDB.ExecContext(context.Background(), `
		CREATE TABLE raw_policy_users (
			id text PRIMARY KEY,
			email text NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	err = db.Raw(context.Background(), `SELECT id, email FROM raw_policy_users WHERE email = ?`, "alice@example.com").Scan(&[]rawIntegrationUser{})
	if err == nil {
		t.Fatal("expected policy error")
	}
	if !errors.Is(err, dormerrors.ErrRawSQLPolicyRequired) {
		t.Fatalf("expected raw SQL policy sentinel, got %T %v", err, err)
	}
}

func TestIntegrationRawSQLObservability(t *testing.T) {
	project := itest.NewProject(t)
	provider := &integrationRawTracerProvider{}
	db := project.OpenDB(t, orm.ObservabilityConfig{
		Tracing:        true,
		TraceSQL:       orm.TraceSQLStatement,
		TracerProvider: provider,
	}, false)
	sqlDB := project.OpenSQL(t)
	defer sqlDB.Close()

	_, err := sqlDB.ExecContext(context.Background(), `
		CREATE TABLE raw_trace_users (
			id text PRIMARY KEY,
			email text NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = sqlDB.ExecContext(context.Background(), `INSERT INTO raw_trace_users (id, email) VALUES ($1, $2)`, "user-1", "alice@example.com")
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}

	var users []rawIntegrationUser
	if err := db.Raw(context.Background(), `SELECT id, email FROM raw_trace_users WHERE email = ?`, "alice@example.com").
		WithoutPolicy().
		Scan(&users); err != nil {
		t.Fatalf("raw scan: %v", err)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	sawRaw := false
	for _, span := range provider.spans {
		if span.Name == "db.raw.scan" {
			sawRaw = true
			if got := span.Attributes["orm.raw"]; got != true {
				t.Fatalf("expected raw attribute, got %#v", span.Attributes)
			}
			if got := span.Attributes["db.statement"]; got == "" {
				t.Fatalf("expected statement attribute, got %#v", span.Attributes)
			}
		}
	}
	if !sawRaw {
		t.Fatalf("expected raw scan span, got %#v", provider.spans)
	}
}
