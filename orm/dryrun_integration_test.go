package orm_test

import (
	"context"
	"testing"
	"time"

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/internal/itest"
	"github.com/dionisius77/dorm/orm"
)

type dryRunIntegrationProduct struct {
	ID         string
	CompanyID  string
	SKU        string
	Name       string
	PriceCents int64
	DeletedAt  *time.Time
	CreatedAt  time.Time
	CreatedBy  string
	UpdatedAt  time.Time
	UpdatedBy  string
	DeletedBy  string
}

func TestDryRunDoesNotMutatePostgres(t *testing.T) {
	project := itest.NewProject(t)
	project.WriteFile(t, "models/models.go", itest.DefaultModelSource)
	project.SaveSnapshot(t, project.BuildSchema(t))

	db := project.OpenDB(t, orm.ObservabilityConfig{}, false)
	sqlDB := db.SQLDB()

	if _, err := sqlDB.ExecContext(context.Background(), `
		INSERT INTO products (id, company_id, sku, name, price_cents)
		VALUES ($1, $2, $3, $4, $5)
	`, "product-1", project.Schema, "SKU-1", "Widget", 100); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	var before int
	if err := sqlDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM products`).Scan(&before); err != nil {
		t.Fatalf("count before dry-run: %v", err)
	}

	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		UserID:    "user-1",
		CompanyID: project.Schema,
	}), access.Default())

	report, err := db.WithContext(ctx).DryRun().Create(ctx, &dryRunIntegrationProduct{
		ID:         "product-2",
		CompanyID:  project.Schema,
		SKU:        "SKU-2",
		Name:       "Gadget",
		PriceCents: 250,
	})
	if err != nil {
		t.Fatalf("dry-run create: %v", err)
	}
	if report.ExecutionStatus != orm.ExecutionStatusSkipped {
		t.Fatalf("expected skipped execution, got %q", report.ExecutionStatus)
	}

	var after int
	if err := sqlDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM products`).Scan(&after); err != nil {
		t.Fatalf("count after dry-run: %v", err)
	}
	if before != after {
		t.Fatalf("expected no database mutation, before=%d after=%d", before, after)
	}
}
