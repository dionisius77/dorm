package access

import (
	"context"
	"errors"
	"testing"

	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/schema"
)

func TestEngineInjectsCompanyScope(t *testing.T) {
	table := &schema.Table{
		Name: "products",
		Columns: []*schema.Column{
			{Name: "company_id", Scope: schema.ScopeCompany},
			{Name: "name"},
		},
	}
	ctx := WithContext(context.Background(), Context{CompanyID: "company-1", UserID: "user-1"})
	preds, writes, err := NewEngine().Apply(ctx, table, OpInsert, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) == 0 {
		t.Fatalf("expected write injection")
	}
	preds, _, err = NewEngine().Apply(ctx, table, OpQuery, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preds) == 0 {
		t.Fatalf("expected predicate injection")
	}
}

func TestEngineRequiresContextForScopedModels(t *testing.T) {
	table := &schema.Table{
		Name: "products",
		Columns: []*schema.Column{
			{Name: "company_id", Scope: schema.ScopeCompany},
			{Name: "name"},
		},
	}
	_, _, err := NewEngine().Apply(context.Background(), table, OpQuery, nil)
	if err == nil {
		t.Fatalf("expected missing company context error")
	}
	if _, ok := err.(*MissingContextError); !ok {
		t.Fatalf("expected MissingContextError, got %T", err)
	}
	if !errors.Is(err, errkind.ErrAccessViolation) {
		t.Fatalf("expected access violation error, got %T %v", err, err)
	}
}

func TestEngineRequiresUserContextForAuditFields(t *testing.T) {
	table := &schema.Table{
		Name: "products",
		Columns: []*schema.Column{
			{Name: "created_by", CreatedBy: true},
			{Name: "updated_by", UpdatedBy: true},
			{Name: "deleted_by", DeletedBy: true},
		},
	}
	_, _, err := NewEngine().Apply(context.Background(), table, OpInsert, nil)
	if err == nil {
		t.Fatalf("expected missing user context error")
	}
	if _, ok := err.(*MissingContextError); !ok {
		t.Fatalf("expected MissingContextError, got %T", err)
	}
}
