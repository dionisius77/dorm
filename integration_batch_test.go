package dorm_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/orm"
)

func TestIntegrationBatchCreateUpdateDelete(t *testing.T) {
	project, _ := prepareMigratedProject(t)

	provider := &integrationHookTracerProvider{}
	db := project.OpenDB(t, orm.ObservabilityConfig{
		Tracing:        true,
		TraceSQL:       orm.TraceSQLStatement,
		TracerProvider: provider,
	}, true)

	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	}), access.Default())
	ctx = context.WithValue(ctx, integrationHookContextKey{}, "batch-trace")

	recorder := &integrationHookRecorder{}
	integrationHookRecorderInstance = recorder
	t.Cleanup(func() {
		integrationHookRecorderInstance = nil
	})

	products := []*Product{
		{
			ID:         "batch-1",
			SKU:        "BATCH-001",
			Name:       "Alpha",
			PriceCents: 100,
		},
		{
			ID:         "batch-2",
			SKU:        "BATCH-002",
			Name:       "Beta",
			PriceCents: 200,
		},
	}
	if err := db.CreateMany(ctx, products); err != nil {
		t.Fatalf("create many: %v", err)
	}
	if got, want := hookNamesFromIntegrationCalls(recorder.entries()), []string{"BeforeCreate", "AfterCreate", "BeforeCreate", "AfterCreate"}; !equalStringSlices(got, want) {
		t.Fatalf("unexpected create hook order: got %v want %v", got, want)
	}
	if !hasIntegrationSpanEvent(provider.spans, "db.batch", "batch.start") || !hasIntegrationSpanEvent(provider.spans, "db.batch", "batch.complete") {
		t.Fatalf("expected batch span events, got %#v", provider.spans)
	}
	for _, p := range products {
		if p.CompanyID != "company-a" || p.CreatedBy != "user-a" || p.UpdatedBy != "user-a" {
			t.Fatalf("expected audit and scope fields to be populated: %#v", p)
		}
		if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
			t.Fatalf("expected timestamps to be populated: %#v", p)
		}
	}

	integrationHookRecorderInstance = nil
	var rows []Product
	if err := db.WithContext(ctx).Find(&rows, orm.OrderBy("sku ASC")); err != nil {
		t.Fatalf("find created rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 created rows, got %#v", rows)
	}
	if rows[0].CompanyID != "company-a" || rows[1].CompanyID != "company-a" {
		t.Fatalf("expected company scope to persist, got %#v", rows)
	}

	recorder = &integrationHookRecorder{}
	integrationHookRecorderInstance = recorder
	products[0].Name = "Alpha Prime"
	products[1].Name = "Beta Prime"
	if err := db.UpdateMany(ctx, products); err != nil {
		t.Fatalf("update many: %v", err)
	}
	if got, want := hookNamesFromIntegrationCalls(recorder.entries()), []string{"BeforeUpdate", "AfterUpdate", "BeforeUpdate", "AfterUpdate"}; !equalStringSlices(got, want) {
		t.Fatalf("unexpected update hook order: got %v want %v", got, want)
	}

	integrationHookRecorderInstance = nil
	rows = nil
	if err := db.WithContext(ctx).Find(&rows, orm.OrderBy("sku ASC")); err != nil {
		t.Fatalf("find updated rows: %v", err)
	}
	if len(rows) != 2 || rows[0].Name != "Alpha Prime" || rows[1].Name != "Beta Prime" {
		t.Fatalf("expected updated names, got %#v", rows)
	}
	if rows[0].UpdatedBy != "user-a" || rows[1].UpdatedBy != "user-a" {
		t.Fatalf("expected update audit fields to persist, got %#v", rows)
	}

	recorder = &integrationHookRecorder{}
	integrationHookRecorderInstance = recorder
	if err := db.DeleteMany(ctx, products); err != nil {
		t.Fatalf("delete many: %v", err)
	}
	if got, want := hookNamesFromIntegrationCalls(recorder.entries()), []string{"BeforeDelete", "AfterDelete", "BeforeDelete", "AfterDelete"}; !equalStringSlices(got, want) {
		t.Fatalf("unexpected delete hook order: got %v want %v", got, want)
	}

	integrationHookRecorderInstance = nil
	rows = nil
	if err := db.WithContext(ctx).WithDeleted().Find(&rows, orm.OrderBy("sku ASC")); err != nil {
		t.Fatalf("find deleted rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 deleted rows, got %#v", rows)
	}
	for _, row := range rows {
		if row.DeletedAt == nil || row.DeletedBy != "user-a" {
			t.Fatalf("expected soft delete audit fields, got %#v", row)
		}
	}
}

func TestIntegrationBatchCreateManyRollbackOnFailure(t *testing.T) {
	project, _ := prepareMigratedProject(t)

	db := project.OpenDB(t, orm.ObservabilityConfig{}, true)
	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	}), access.Default())

	products := []*Product{
		{
			ID:         "rollback-1",
			SKU:        "ROLLBACK-001",
			Name:       "ok",
			PriceCents: 100,
		},
		{
			ID:         "rollback-2",
			SKU:        "ROLLBACK-002",
			Name:       "fail-before-create",
			PriceCents: 200,
		},
	}
	err := db.CreateMany(ctx, products)
	if err == nil {
		t.Fatal("expected create many failure")
	}
	if !errors.Is(err, integrationHookFailure) {
		t.Fatalf("expected hook failure cause, got %v", err)
	}
	if !strings.Contains(err.Error(), "create_many[1]") {
		t.Fatalf("expected batch item context in error, got %v", err)
	}

	var rows []Product
	if err := db.WithContext(ctx).WithDeleted().Find(&rows, orm.Where("sku LIKE ?", "ROLLBACK-%")); err != nil {
		t.Fatalf("check rollback rows: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected rollback to remove batch rows, got %#v", rows)
	}
}

func hookNamesFromIntegrationCalls(calls []integrationHookCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, call.Name)
	}
	return out
}

func hasIntegrationSpanEvent(spans []integrationHookSpanRecord, spanName, eventName string) bool {
	for _, span := range spans {
		if span.Name != spanName {
			continue
		}
		for _, event := range span.Events {
			if event == eventName {
				return true
			}
		}
	}
	return false
}
