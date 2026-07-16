package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/dionisius77/dorm/access"
)

func TestBatchCreateManySuccess(t *testing.T) {
	scenario := t.Name()
	hookResetState(scenario)
	provider := &hookTraceProvider{}
	db, state := hookTestDB(t, scenario, ObservabilityConfig{
		Tracing:        true,
		TracerProvider: provider,
	})
	hookTestRecorderInstance = &hookRecorder{state: state}
	t.Cleanup(func() {
		hookTestRecorderInstance = nil
	})
	db.batchSize = 1

	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	}), access.Default())
	ctx = context.WithValue(ctx, hookTestContextKey{}, "trace-batch")

	items := []*hookModel{
		{ID: "1", Name: "alpha"},
		{ID: "2", Name: "beta"},
	}
	if err := db.CreateMany(ctx, items); err != nil {
		t.Fatalf("create many: %v", err)
	}
	if state.beginCount != 1 || state.commitCount != 1 || state.rollbackCount != 0 {
		t.Fatalf("unexpected transaction counts: %+v", state)
	}
	calls := hookRecorderEntries(state)
	if got, want := batchHookCallNames(calls), []string{"BeforeCreate", "AfterCreate", "BeforeCreate", "AfterCreate"}; !batchEqualStringSlices(got, want) {
		t.Fatalf("unexpected hook order: got %v want %v", got, want)
	}
	if items[0].CompanyID != "company-a" || items[1].CompanyID != "company-a" {
		t.Fatalf("expected company scope to be injected: %#v", items)
	}
	if items[0].CreatedAt.IsZero() || items[0].UpdatedAt.IsZero() || items[1].CreatedAt.IsZero() || items[1].UpdatedAt.IsZero() {
		t.Fatalf("expected audit timestamps to be populated: %#v", items)
	}
	if !hasSpanEvent(provider.spans, "db.batch", "batch.start") || !hasSpanEvent(provider.spans, "db.batch", "batch.complete") {
		t.Fatalf("expected batch span events, got %#v", provider.spans)
	}
}

func TestBatchCreateManyRollbackOnHookFailure(t *testing.T) {
	scenario := t.Name()
	hookResetState(scenario)
	provider := &hookTraceProvider{}
	db, state := hookTestDB(t, scenario, ObservabilityConfig{
		Tracing:        true,
		TracerProvider: provider,
	})
	hookTestRecorderInstance = &hookRecorder{state: state}
	t.Cleanup(func() {
		hookTestRecorderInstance = nil
	})
	db.batchSize = 1

	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	}), access.Default())

	items := []*hookModel{
		{ID: "1", Name: "alpha"},
		{ID: "2", Name: "beta", FailOn: "BeforeCreate"},
	}
	err := db.CreateMany(ctx, items)
	if err == nil {
		t.Fatal("expected create many failure")
	}
	if !errors.Is(err, hookFailure) {
		t.Fatalf("expected hook failure cause, got %v", err)
	}
	if state.beginCount != 1 || state.commitCount != 0 || state.rollbackCount != 1 {
		t.Fatalf("unexpected transaction counts: %+v", state)
	}
	calls := hookRecorderEntries(state)
	if got, want := batchHookCallNames(calls), []string{"BeforeCreate", "AfterCreate", "BeforeCreate"}; !batchEqualStringSlices(got, want) {
		t.Fatalf("unexpected hook order: got %v want %v", got, want)
	}
	if !hasSpanEvent(provider.spans, "db.batch", "batch.error") {
		t.Fatalf("expected batch error event, got %#v", provider.spans)
	}
}

func TestBatchUpdateAndDeleteManySuccess(t *testing.T) {
	scenario := t.Name()
	hookResetState(scenario)
	provider := &hookTraceProvider{}
	db, state := hookTestDB(t, scenario, ObservabilityConfig{
		Tracing:        true,
		TracerProvider: provider,
	})
	hookTestRecorderInstance = &hookRecorder{state: state}
	t.Cleanup(func() {
		hookTestRecorderInstance = nil
	})
	db.batchSize = 1

	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	}), access.Default())

	items := []*hookModel{
		{ID: "1", Name: "alpha"},
		{ID: "2", Name: "beta"},
	}
	if err := db.CreateMany(ctx, items); err != nil {
		t.Fatalf("seed create many: %v", err)
	}

	hookResetState(scenario)
	state = hookTestStateFor(scenario)
	hookTestRecorderInstance = &hookRecorder{state: state}
	items[0].Name = "alpha-updated"
	items[1].Name = "beta-updated"
	if err := db.UpdateMany(ctx, items); err != nil {
		t.Fatalf("update many: %v", err)
	}
	if state.beginCount != 1 || state.commitCount != 1 || state.rollbackCount != 0 {
		t.Fatalf("unexpected update transaction counts: %+v", state)
	}
	calls := hookRecorderEntries(state)
	if got, want := batchHookCallNames(calls), []string{"BeforeUpdate", "AfterUpdate", "BeforeUpdate", "AfterUpdate"}; !batchEqualStringSlices(got, want) {
		t.Fatalf("unexpected update hook order: got %v want %v", got, want)
	}
	if items[0].UpdatedAt.IsZero() || items[1].UpdatedAt.IsZero() {
		t.Fatalf("expected update audit timestamps to be populated: %#v", items)
	}

	hookResetState(scenario)
	state = hookTestStateFor(scenario)
	hookTestRecorderInstance = &hookRecorder{state: state}
	if err := db.DeleteMany(ctx, items); err != nil {
		t.Fatalf("delete many: %v", err)
	}
	if state.beginCount != 1 || state.commitCount != 1 || state.rollbackCount != 0 {
		t.Fatalf("unexpected delete transaction counts: %+v", state)
	}
	calls = hookRecorderEntries(state)
	if got, want := batchHookCallNames(calls), []string{"BeforeDelete", "AfterDelete", "BeforeDelete", "AfterDelete"}; !batchEqualStringSlices(got, want) {
		t.Fatalf("unexpected delete hook order: got %v want %v", got, want)
	}
	if items[0].DeletedAt == nil || items[1].DeletedAt == nil {
		t.Fatalf("expected soft delete timestamps to be populated: %#v", items)
	}
}

func hasSpanEvent(spans []hookTraceSpanRecord, spanName, eventName string) bool {
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

func batchHookCallNames(calls []hookCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, call.Name)
	}
	return out
}

func batchEqualStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
