package access

import (
	"context"
	"testing"

	"github.com/dionisius77/dorm/schema"
)

func TestEngineHonorsIgnoreCompanyPolicy(t *testing.T) {
	table := &schema.Table{
		Name: "products",
		Columns: []*schema.Column{
			{Name: "company_id", Scope: schema.ScopeCompany},
			{Name: "tenant_id", Scope: schema.ScopeTenant},
		},
	}
	ctx := WithPolicy(WithContext(context.Background(), Context{TenantID: "tenant-1"}), IgnoreCompany())
	preds, writes, err := NewEngine().Apply(ctx, table, OpQuery, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preds) != 1 || preds[0].SQL != "tenant_id = ?" {
		t.Fatalf("expected only tenant predicate, got %#v", preds)
	}
	preds, writes, err = NewEngine().Apply(ctx, table, OpInsert, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 || writes[0].Field != "tenant_id" {
		t.Fatalf("expected only tenant write, got %#v", writes)
	}
}

func TestEngineHonorsIgnoreRLSPolicy(t *testing.T) {
	table := &schema.Table{
		Name: "products",
		Columns: []*schema.Column{
			{Name: "company_id", Scope: schema.ScopeCompany},
			{Name: "created_by", CreatedBy: true},
		},
	}
	ctx := WithPolicy(WithContext(context.Background(), Context{UserID: "user-1"}), IgnoreRLS())
	preds, writes, err := NewEngine().Apply(ctx, table, OpQuery, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preds) != 0 {
		t.Fatalf("expected no row-level predicates, got %#v", preds)
	}
	_, writes, err = NewEngine().Apply(ctx, table, OpInsert, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 || writes[0].Field != "created_by" {
		t.Fatalf("expected audit write only, got %#v", writes)
	}
}

func TestEngineHonorsSystemPolicy(t *testing.T) {
	table := &schema.Table{
		Name: "products",
		Columns: []*schema.Column{
			{Name: "company_id", Scope: schema.ScopeCompany},
			{Name: "created_by", CreatedBy: true},
		},
	}
	ctx := WithPolicy(context.Background(), System())
	preds, writes, err := NewEngine().Apply(ctx, table, OpInsert, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preds) != 0 || len(writes) != 0 {
		t.Fatalf("expected all policies disabled, got preds=%#v writes=%#v", preds, writes)
	}
}
