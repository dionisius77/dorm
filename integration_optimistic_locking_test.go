package dorm_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/dionisius77/dorm"
	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/internal/itest"
	"github.com/dionisius77/dorm/model"
	"github.com/dionisius77/dorm/orm"
)

type integrationOptimisticLockTracerProvider struct {
	mu    sync.Mutex
	spans []string
}

type integrationOptimisticLockTracer struct {
	provider *integrationOptimisticLockTracerProvider
}

type integrationOptimisticLockSpan struct{}

func (p *integrationOptimisticLockTracerProvider) Tracer(string) orm.Tracer {
	return integrationOptimisticLockTracer{provider: p}
}

func (t integrationOptimisticLockTracer) Start(ctx context.Context, name string, _ ...orm.SpanOption) (context.Context, orm.Span) {
	t.provider.mu.Lock()
	t.provider.spans = append(t.provider.spans, name)
	t.provider.mu.Unlock()
	return ctx, integrationOptimisticLockSpan{}
}

func (integrationOptimisticLockSpan) End() {}

func (integrationOptimisticLockSpan) RecordError(error) {}

func (integrationOptimisticLockSpan) SetAttributes(...orm.Attribute) {}

type integrationVersionedUser struct {
	model.Company
	model.Version
	ID   string `orm:"pk"`
	Name string
}

func TestIntegrationOptimisticLocking(t *testing.T) {
	project := itest.NewProject(t)
	sqlDB := project.OpenSQL(t)
	defer sqlDB.Close()

	if _, err := sqlDB.ExecContext(context.Background(), `
		CREATE TABLE versioned_users (
			id text PRIMARY KEY,
			company_id text NOT NULL,
			name text NOT NULL,
			version bigint NOT NULL DEFAULT 1
		)
	`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	if _, err := sqlDB.ExecContext(context.Background(), `
		INSERT INTO versioned_users (id, company_id, name)
		VALUES ($1, $2, $3)
	`, "user-1", "company-a", "Alpha"); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	provider := &integrationOptimisticLockTracerProvider{}
	db := project.OpenDB(t, orm.ObservabilityConfig{
		Tracing:        true,
		TracerProvider: provider,
	}, false)

	ctx := access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	})

	current := &integrationVersionedUser{
		Company: model.Company{CompanyID: "company-a"},
		Version: model.Version{Version: 1},
		ID:      "user-1",
		Name:    "Alpha Updated",
	}
	if err := db.Transaction(ctx, func(tx *orm.DB) error {
		return tx.Update(current)
	}); err != nil {
		t.Fatalf("transaction update: %v", err)
	}
	if current.Version.Version != 2 {
		t.Fatalf("expected refreshed version 2, got %#v", current.Version)
	}

	var persisted integrationVersionedUser
	if err := sqlDB.QueryRowContext(context.Background(), `
		SELECT id, company_id, name, version
		FROM versioned_users
		WHERE id = $1
	`, "user-1").Scan(&persisted.ID, &persisted.CompanyID, &persisted.Name, &persisted.Version.Version); err != nil {
		t.Fatalf("load persisted row: %v", err)
	}
	if persisted.Version.Version != 2 {
		t.Fatalf("expected persisted version 2, got %#v", persisted.Version)
	}

	stale := &integrationVersionedUser{
		Company: model.Company{CompanyID: "company-a"},
		Version: model.Version{Version: 1},
		ID:      "user-1",
		Name:    "Stale Update",
	}
	if err := db.WithContext(ctx).Update(stale); !errors.Is(err, dorm.ErrOptimisticLockFailed) {
		t.Fatalf("expected optimistic lock failure, got %T %v", err, err)
	}

	report, err := db.WithContext(ctx).DryRun().Update(ctx, &integrationVersionedUser{
		Company: model.Company{CompanyID: "company-a"},
		Version: model.Version{Version: 2},
		ID:      "user-1",
		Name:    "Dry Run Update",
	})
	if err != nil {
		t.Fatalf("dry-run update: %v", err)
	}
	if report.OptimisticLocking == nil || !report.OptimisticLocking.Enabled {
		t.Fatalf("expected optimistic-lock metadata, got %#v", report.OptimisticLocking)
	}
	if report.OptimisticLocking.Current != int64(2) || report.OptimisticLocking.Next != int64(3) {
		t.Fatalf("unexpected optimistic-lock metadata: %#v", report.OptimisticLocking)
	}
	foundInspectSpan := false
	for _, span := range provider.spans {
		if strings.HasPrefix(span, "db.inspect.") {
			foundInspectSpan = true
			break
		}
	}
	if !foundInspectSpan {
		t.Fatalf("expected inspection spans, got %#v", provider.spans)
	}

	if err := sqlDB.QueryRowContext(context.Background(), `
		SELECT version
		FROM versioned_users
		WHERE id = $1
	`, "user-1").Scan(&persisted.Version.Version); err != nil {
		t.Fatalf("reload version: %v", err)
	}
	if persisted.Version.Version != 2 {
		t.Fatalf("expected dry-run to avoid mutation, got version %d", persisted.Version.Version)
	}
}
