package access

import (
	"context"
	"testing"

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
