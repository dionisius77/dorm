package orm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/model"
	"github.com/dionisius77/dorm/schema"
)

type dryRunTestUser struct {
	ID        string
	CompanyID string
	TenantID  string
	Name      string
	DeletedAt *time.Time
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	model.Version
}

func (u *dryRunTestUser) BeforeFind(context.Context) error { return nil }

func (u *dryRunTestUser) BeforeCreate(context.Context, *DB) error { return nil }

func (u *dryRunTestUser) AfterCreate(context.Context, *DB) error { return nil }

type dryRunAdvisorStub struct {
	called bool
	input  QueryAdvisorInput
}

func (a *dryRunAdvisorStub) Inspect(_ context.Context, input QueryAdvisorInput) (QueryAdvisorReport, error) {
	a.called = true
	a.input = input
	return QueryAdvisorReport{
		Table: input.Table,
		SQL:   input.SQL,
		Kind:  input.Operation,
		Findings: []QueryAdvisorFinding{
			{
				Code:           "missing_composite_index",
				Title:          "Missing composite index",
				Severity:       "warning",
				Recommendation: "(company_id, deleted_at)",
			},
		},
	}, nil
}

func TestDryRunFindReturnsInspectionReport(t *testing.T) {
	db := dryRunTestDB(t, &dryRunAdvisorStub{}, &rawTestTracerProvider{})
	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		UserID:    "user-1",
		CompanyID: "company-1",
		TenantID:  "tenant-1",
	}), access.Default())

	var users []dryRunTestUser
	report, err := db.WithContext(ctx).DryRun().Find(ctx, &users, Where("id = ?", "user-1"))
	if err != nil {
		t.Fatalf("dry run find: %v", err)
	}
	if report.ExecutionStatus != ExecutionStatusSkipped {
		t.Fatalf("expected skipped execution, got %q", report.ExecutionStatus)
	}
	if !strings.Contains(strings.ToUpper(report.SQL), "SELECT") {
		t.Fatalf("expected generated SQL in report, got %q", report.SQL)
	}
	if len(report.Parameters) != 3 {
		t.Fatalf("expected bound parameters, got %#v", report.Parameters)
	}
	if report.Parameters[0] != "user-1" || report.Parameters[1] != "company-1" || report.Parameters[2] != "tenant-1" {
		t.Fatalf("unexpected parameters: %#v", report.Parameters)
	}
	if !hasAccessEvent(report.AccessPolicies, AccessPolicyEventSoftDelete, "deleted_at") {
		t.Fatalf("expected soft delete access event, got %#v", report.AccessPolicies)
	}
	if !hasAccessEvent(report.AccessPolicies, AccessPolicyEventInjectedPredicate, "company_id") {
		t.Fatalf("expected company predicate, got %#v", report.AccessPolicies)
	}
	if !hasHookEvent(report.LifecycleHooks, "BeforeFind") {
		t.Fatalf("expected before find hook, got %#v", report.LifecycleHooks)
	}
	if len(report.QueryAdvisor) != 1 {
		t.Fatalf("expected query advisor findings, got %#v", report.QueryAdvisor)
	}
}

func TestDryRunCreateReportsAuditAndHooks(t *testing.T) {
	advisor := &dryRunAdvisorStub{}
	db := dryRunTestDB(t, advisor, &rawTestTracerProvider{})
	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		UserID:    "user-1",
		CompanyID: "company-1",
		TenantID:  "tenant-1",
	}), access.Default())

	user := &dryRunTestUser{ID: "user-1", Name: "Alice"}
	report, err := db.WithContext(ctx).DryRun().Create(ctx, user)
	if err != nil {
		t.Fatalf("dry run create: %v", err)
	}
	for _, want := range []string{"created_by", "updated_by", "created_at", "updated_at"} {
		if !hasAuditField(report.AuditActions, want) {
			t.Fatalf("expected audit field %q, got %#v", want, report.AuditActions)
		}
	}
	if !hasAccessEvent(report.AccessPolicies, AccessPolicyEventInjectedField, "company_id") {
		t.Fatalf("expected company field injection, got %#v", report.AccessPolicies)
	}
	if !hookOrder(report.LifecycleHooks, []string{"BeforeCreate", "AfterCreate"}) {
		t.Fatalf("unexpected hook order: %#v", report.LifecycleHooks)
	}
	if !advisor.called || advisor.input.Operation != "create" {
		t.Fatalf("expected advisor to be called for create, got %#v", advisor.input)
	}
}

func TestDryRunInsideTransactionDoesNotExecuteSQL(t *testing.T) {
	scenario := t.Name()
	transactionTestResetState(scenario)
	db, state := transactionTestDB(t, scenario, ObservabilityConfig{})
	ctx := access.WithContext(context.Background(), access.Context{
		UserID:    "user-1",
		CompanyID: "company-1",
	})
	rollbackErr := errors.New("force rollback")
	err := db.Transaction(ctx, func(tx *DB) error {
		var rows []txTestProduct
		_, findErr := tx.DryRun().Find(ctx, &rows, Where("sku = ?", "SKU-001"))
		if findErr != nil {
			return findErr
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.queries) != 0 {
		t.Fatalf("expected no SQL execution, got %+v", state)
	}
	if state.beginCount != 1 || state.rollbackCount != 1 || state.commitCount != 0 {
		t.Fatalf("unexpected transaction lifecycle, got %+v", state)
	}
}

func TestDryRunObservabilityUsesInspectionSpans(t *testing.T) {
	provider := &rawTestTracerProvider{}
	db := dryRunTestDB(t, nil, provider)
	ctx := access.WithContext(context.Background(), access.Context{
		CompanyID: "company-1",
		TenantID:  "tenant-1",
	})
	var rows []dryRunTestUser
	if _, err := db.WithContext(ctx).DryRun().Find(ctx, &rows, Where("id = ?", "user-1")); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, span := range provider.spans {
		if strings.HasPrefix(span.Name, "db.inspect.") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected inspection spans, got %#v", provider.spans)
	}
}

func TestDryRunUpdateReportsOptimisticLocking(t *testing.T) {
	db := dryRunTestDB(t, nil, &rawTestTracerProvider{})
	ctx := access.WithContext(context.Background(), access.Context{
		CompanyID: "company-1",
		UserID:    "user-1",
		TenantID:  "tenant-1",
	})
	user := &dryRunTestUser{
		ID:      "user-1",
		Name:    "Alice",
		Version: model.Version{Version: 3},
	}
	report, err := db.WithContext(ctx).DryRun().Update(ctx, user)
	if err != nil {
		t.Fatalf("dry run update: %v", err)
	}
	if report.OptimisticLocking == nil || !report.OptimisticLocking.Enabled {
		t.Fatalf("expected optimistic locking metadata, got %#v", report.OptimisticLocking)
	}
	if report.OptimisticLocking.Current != int64(3) {
		t.Fatalf("expected current version 3, got %#v", report.OptimisticLocking.Current)
	}
	if report.OptimisticLocking.Next != int64(4) {
		t.Fatalf("expected next version 4, got %#v", report.OptimisticLocking.Next)
	}
	if !strings.Contains(strings.ToLower(report.SQL), `"version" = "version" + 1`) {
		t.Fatalf("expected version increment in SQL, got %q", report.SQL)
	}
	if !strings.Contains(strings.ToLower(report.SQL), `"version" = $`) {
		t.Fatalf("expected version predicate in SQL, got %q", report.SQL)
	}
}

func dryRunTestDB(t *testing.T, advisor QueryAdvisor, tracer TracerProvider) *DB {
	t.Helper()
	table := &schema.Table{
		Name:       "users",
		GoTypeName: "dryRunTestUser",
		Columns: []*schema.Column{
			{Name: "id", PrimaryKey: true},
			{Name: "company_id", Scope: schema.ScopeCompany},
			{Name: "tenant_id", Scope: schema.ScopeTenant},
			{Name: "name"},
			{Name: "deleted_at", SoftDelete: true},
			{Name: "created_at", CreatedAt: true},
			{Name: "created_by", CreatedBy: true},
			{Name: "updated_at", UpdatedAt: true},
			{Name: "updated_by", UpdatedBy: true},
			{Name: "version", Version: true},
		},
	}
	return New(Config{
		Dialect: postgres.New(),
		Schema: &schema.Schema{
			Tables: []*schema.Table{table},
		},
		QueryAdvisor: advisor,
		Observability: ObservabilityConfig{
			Tracing:        tracer != nil,
			TracerProvider: tracer,
			TraceSQL:       TraceSQLStatementWithArgs,
		},
	})
}

func hasAccessEvent(events []AccessPolicyEvent, kind AccessPolicyEventKind, field string) bool {
	for _, event := range events {
		if event.Kind == kind && event.Field == field {
			return true
		}
	}
	return false
}

func hasAuditField(actions []AuditAction, field string) bool {
	for _, action := range actions {
		if action.Field == field {
			return true
		}
	}
	return false
}

func hasHookEvent(events []LifecycleHookEvent, name string) bool {
	for _, event := range events {
		if event.Name == name {
			return true
		}
	}
	return false
}

func hookOrder(events []LifecycleHookEvent, names []string) bool {
	if len(events) != len(names) {
		return false
	}
	for i, event := range events {
		if event.Name != names[i] {
			return false
		}
	}
	return true
}

var _ QueryAdvisor = (*dryRunAdvisorStub)(nil)
