package orm

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/schema"
)

func TestSessionWithPolicyPreservesContextPolicy(t *testing.T) {
	db := New(Config{})
	ctx := access.WithContext(context.Background(), access.Context{
		UserID:    "user-1",
		CompanyID: "company-1",
	})
	session := db.WithPolicy(access.IgnoreCompany()).WithContext(ctx)
	if got := access.PolicyFromContext(session.ctx); got.Level != access.PolicyLevelIgnoreCompany {
		t.Fatalf("expected ignore_company policy, got %q", got.Level)
	}
}

func TestBuildStateHonorsSystemPolicySoftDeleteBypass(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}
	db := New(Config{
		Schema: &schema.Schema{
			Tables: []*schema.Table{
				{
					Name:       "users",
					GoTypeName: "User",
					Columns: []*schema.Column{
						{Name: "id", PrimaryKey: true},
						{Name: "name"},
						{Name: "deleted_at", DeletedAt: true, SoftDelete: true},
						{Name: "company_id", Scope: schema.ScopeCompany},
					},
				},
			},
		},
	})

	defaultSession := db.WithContext(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-1",
		UserID:    "user-1",
	}))
	var users []User
	state, err := defaultSession.buildState(&users)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.where) == 0 {
		t.Fatalf("expected where clauses for default policy")
	}

	systemSession := db.WithPolicy(access.System())
	state, err = systemSession.buildState(&users)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.where) != 0 {
		t.Fatalf("expected no policy-derived where clauses under system policy, got %#v", state.where)
	}
}

func TestColumnsFromTypeFlattensEmbeddedTraits(t *testing.T) {
	type Company struct {
		CompanyID string
	}
	type Audit struct {
		CreatedAt time.Time
		CreatedBy string
	}
	type Entity struct {
		Company
		Audit
	}
	type Invoice struct {
		Entity
		ID     int
		Number string
	}
	cols := columnsFromType(reflect.TypeOf(Invoice{}))
	if len(cols) == 0 {
		t.Fatal("expected columns")
	}
	found := map[string]*schema.Column{}
	for _, col := range cols {
		found[col.Name] = col
	}
	if found["company_id"] == nil || found["company_id"].Scope != schema.ScopeCompany {
		t.Fatalf("expected company_id scope, got %#v", found["company_id"])
	}
	if found["created_at"] == nil || !found["created_at"].CreatedAt {
		t.Fatalf("expected created_at audit flag, got %#v", found["created_at"])
	}
	if found["created_by"] == nil || !found["created_by"].CreatedBy {
		t.Fatalf("expected created_by audit flag, got %#v", found["created_by"])
	}
}
