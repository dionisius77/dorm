package dorm_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/orm"
)

type integrationHookCall struct {
	Name      string
	CompanyID string
	Policy    string
	TraceID   string
	Tx        bool
}

type integrationHookRecorder struct {
	mu    sync.Mutex
	calls []integrationHookCall
}

type integrationHookTracerProvider struct {
	mu    sync.Mutex
	spans []integrationHookSpanRecord
}

type integrationHookSpanRecord struct {
	Name    string
	Events  []string
	Errored bool
}

type integrationHookTracer struct {
	provider *integrationHookTracerProvider
}

type integrationHookSpan struct {
	provider *integrationHookTracerProvider
	index    int
}

var integrationHookFailure = errors.New("integration hook failure")

func (r *integrationHookRecorder) add(name string, ctx context.Context, tx *orm.DB) {
	if r == nil {
		return
	}
	ac, _ := access.FromContext(ctx)
	call := integrationHookCall{
		Name:      name,
		CompanyID: fmt.Sprint(ac.CompanyID),
		Policy:    access.PolicyFromContext(ctx).Name(),
		Tx:        tx != nil,
	}
	if v, ok := ctx.Value(integrationHookContextKey{}).(string); ok {
		call.TraceID = v
	}
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
}

func (r *integrationHookRecorder) entries() []integrationHookCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]integrationHookCall(nil), r.calls...)
}

func (p *integrationHookTracerProvider) Tracer(string) orm.Tracer {
	return integrationHookTracer{provider: p}
}

func (t integrationHookTracer) Start(ctx context.Context, name string, _ ...orm.SpanOption) (context.Context, orm.Span) {
	t.provider.mu.Lock()
	t.provider.spans = append(t.provider.spans, integrationHookSpanRecord{Name: name})
	idx := len(t.provider.spans) - 1
	t.provider.mu.Unlock()
	return ctx, integrationHookSpan{provider: t.provider, index: idx}
}

func (s integrationHookSpan) End() {}

func (s integrationHookSpan) RecordError(error) {
	s.provider.mu.Lock()
	if s.index >= 0 && s.index < len(s.provider.spans) {
		s.provider.spans[s.index].Errored = true
	}
	s.provider.mu.Unlock()
}

func (s integrationHookSpan) SetAttributes(...orm.Attribute) {}

func (s integrationHookSpan) AddEvent(name string, _ ...orm.Attribute) {
	s.provider.mu.Lock()
	if s.index >= 0 && s.index < len(s.provider.spans) {
		s.provider.spans[s.index].Events = append(s.provider.spans[s.index].Events, name)
	}
	s.provider.mu.Unlock()
}

type integrationHookContextKey struct{}

var integrationHookRecorderInstance *integrationHookRecorder

func (p *Product) BeforeCreate(ctx context.Context, tx *orm.DB) error {
	if integrationHookRecorderInstance != nil {
		integrationHookRecorderInstance.add("BeforeCreate", ctx, tx)
	}
	if p != nil && p.Name == "fail-before-create" {
		return integrationHookFailure
	}
	return nil
}

func (p *Product) AfterCreate(ctx context.Context, tx *orm.DB) error {
	if integrationHookRecorderInstance != nil {
		integrationHookRecorderInstance.add("AfterCreate", ctx, tx)
	}
	return nil
}

func (p *Product) BeforeUpdate(ctx context.Context, tx *orm.DB) error {
	if integrationHookRecorderInstance != nil {
		integrationHookRecorderInstance.add("BeforeUpdate", ctx, tx)
	}
	return nil
}

func (p *Product) AfterUpdate(ctx context.Context, tx *orm.DB) error {
	if integrationHookRecorderInstance != nil {
		integrationHookRecorderInstance.add("AfterUpdate", ctx, tx)
	}
	return nil
}

func (p *Product) BeforeDelete(ctx context.Context, tx *orm.DB) error {
	if integrationHookRecorderInstance != nil {
		integrationHookRecorderInstance.add("BeforeDelete", ctx, tx)
	}
	return nil
}

func (p *Product) AfterDelete(ctx context.Context, tx *orm.DB) error {
	if integrationHookRecorderInstance != nil {
		integrationHookRecorderInstance.add("AfterDelete", ctx, tx)
	}
	return nil
}

func (p *Product) AfterFind(ctx context.Context, tx *orm.DB) error {
	if integrationHookRecorderInstance != nil {
		integrationHookRecorderInstance.add("AfterFind", ctx, tx)
	}
	return nil
}

func TestIntegrationLifecycleHooks(t *testing.T) {
	project, _ := prepareMigratedProject(t)
	recorder := &integrationHookRecorder{}
	integrationHookRecorderInstance = recorder
	t.Cleanup(func() {
		integrationHookRecorderInstance = nil
	})

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
	ctx = context.WithValue(ctx, integrationHookContextKey{}, "integration-trace")

	model := &Product{
		ID:         "hook-product",
		SKU:        "HOOK-PRODUCT",
		Name:       "Hooked",
		PriceCents: 100,
	}

	if err := db.Transaction(ctx, func(tx *orm.DB) error {
		if err := tx.Create(model); err != nil {
			return err
		}
		var rows []Product
		if err := tx.Find(&rows, orm.Where("sku = ?", model.SKU)); err != nil {
			return err
		}
		model.Name = "Hooked Updated"
		if err := tx.Update(model); err != nil {
			return err
		}
		if err := tx.Delete(model); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("hook lifecycle transaction: %v", err)
	}

	calls := recorder.entries()
	names := hookCallNames(calls)
	expected := []string{
		"BeforeCreate",
		"AfterCreate",
		"AfterFind",
		"BeforeUpdate",
		"AfterUpdate",
		"BeforeDelete",
		"AfterDelete",
	}
	if !equalStringSlices(names, expected) {
		t.Fatalf("unexpected hook order: got %v want %v", names, expected)
	}
	for _, call := range calls {
		if call.CompanyID != "company-a" || call.Policy != access.Default().Name() || call.TraceID != "integration-trace" {
			t.Fatalf("unexpected hook context: %#v", call)
		}
		if !call.Tx {
			t.Fatalf("expected transaction handle in hook, got %#v", call)
		}
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	var sawHookSpan, sawEvent bool
	for _, span := range provider.spans {
		if strings.HasPrefix(span.Name, "db.hook.") {
			sawHookSpan = true
		}
		for _, event := range span.Events {
			if event == "BeforeCreate" || event == "AfterFind" {
				sawEvent = true
			}
		}
	}
	if !sawHookSpan || !sawEvent {
		t.Fatalf("expected hook spans and events, got %#v", provider.spans)
	}
}

func TestIntegrationHookFailureRollsBackTransaction(t *testing.T) {
	project, _ := prepareMigratedProject(t)
	recorder := &integrationHookRecorder{}
	integrationHookRecorderInstance = recorder
	t.Cleanup(func() {
		integrationHookRecorderInstance = nil
	})

	db := project.OpenDB(t, orm.ObservabilityConfig{}, true)
	ctx := access.WithPolicy(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	}), access.Default())

	err := db.Transaction(ctx, func(tx *orm.DB) error {
		return tx.Create(&Product{
			ID:         "hook-failure",
			SKU:        "HOOK-FAILURE",
			Name:       "fail-before-create",
			PriceCents: 100,
		})
	})
	if err == nil {
		t.Fatal("expected hook failure")
	}
	if !errors.Is(err, integrationHookFailure) {
		t.Fatalf("expected hook failure cause, got %v", err)
	}
	if !strings.Contains(err.Error(), "BeforeCreate") {
		t.Fatalf("expected lifecycle context in error, got %v", err)
	}

	var rows []Product
	if err := db.WithContext(ctx).WithDeleted().Find(&rows, orm.Where("sku = ?", "HOOK-FAILURE")); err != nil {
		t.Fatalf("check rolled back row: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected hook failure to rollback row, got %#v", rows)
	}
}

func hookCallNames(calls []integrationHookCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, call.Name)
	}
	return out
}

func equalStringSlices(a, b []string) bool {
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
