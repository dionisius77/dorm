package dorm_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/orm"
)

type integrationTransactionTracerProvider struct {
	mu    sync.Mutex
	spans []integrationTransactionSpanRecord
}

type integrationTransactionSpanRecord struct {
	Name    string
	Errored bool
}

type integrationTransactionTracer struct {
	provider *integrationTransactionTracerProvider
}

type integrationTransactionSpan struct {
	provider *integrationTransactionTracerProvider
	index    int
}

func (p *integrationTransactionTracerProvider) Tracer(string) orm.Tracer {
	return integrationTransactionTracer{provider: p}
}

func (t integrationTransactionTracer) Start(ctx context.Context, name string, _ ...orm.SpanOption) (context.Context, orm.Span) {
	t.provider.mu.Lock()
	t.provider.spans = append(t.provider.spans, integrationTransactionSpanRecord{Name: name})
	idx := len(t.provider.spans) - 1
	t.provider.mu.Unlock()
	return ctx, integrationTransactionSpan{provider: t.provider, index: idx}
}

func (s integrationTransactionSpan) End() {}

func (s integrationTransactionSpan) RecordError(error) {
	s.provider.mu.Lock()
	if s.index >= 0 && s.index < len(s.provider.spans) {
		s.provider.spans[s.index].Errored = true
	}
	s.provider.mu.Unlock()
}

func (s integrationTransactionSpan) SetAttributes(...orm.Attribute) {}

func TestIntegrationTransactionLifecycle(t *testing.T) {
	project, _ := prepareMigratedProject(t)
	provider := &integrationTransactionTracerProvider{}
	db := project.OpenDB(t, orm.ObservabilityConfig{
		Tracing:        true,
		TraceSQL:       orm.TraceSQLStatement,
		TracerProvider: provider,
	}, true)

	baseCtx := access.WithContext(context.Background(), access.Context{
		CompanyID: "company-a",
		UserID:    "user-a",
	})

	err := db.Transaction(baseCtx, func(tx *orm.DB) error {
		if err := tx.Create(&Product{
			ID:         "tx-commit",
			SKU:        "TX-COMMIT",
			Name:       "Committed",
			PriceCents: 111,
		}); err != nil {
			return err
		}
		nestedRollbackErr := errors.New("force nested rollback")
		nestedErr := tx.Transaction(baseCtx, func(nested *orm.DB) error {
			if err := nested.Create(&Product{
				ID:         "tx-rollback",
				SKU:        "TX-ROLLBACK",
				Name:       "Rolled back",
				PriceCents: 222,
			}); err != nil {
				return err
			}
			return nestedRollbackErr
		})
		if nestedErr == nil {
			t.Fatal("expected nested rollback error")
		}
		if !errors.Is(nestedErr, nestedRollbackErr) {
			t.Fatalf("expected nested rollback error to be preserved, got %v", nestedErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction commit: %v", err)
	}

	outerRollbackSKU := "TX-ROLLBACK-OUTER"
	if err := db.Transaction(baseCtx, func(tx *orm.DB) error {
		if err := tx.Create(&Product{
			ID:         "tx-outer-rollback",
			SKU:        outerRollbackSKU,
			Name:       "Outer rollback",
			PriceCents: 444,
		}); err != nil {
			return err
		}
		return errors.New("force rollback")
	}); err == nil {
		t.Fatal("expected rollback error")
	}
	var outerRolledBack []Product
	if err := db.WithContext(baseCtx).Find(&outerRolledBack, orm.Where("sku = ?", outerRollbackSKU)); err != nil {
		t.Fatalf("find rolled back outer row: %v", err)
	}
	if len(outerRolledBack) != 0 {
		t.Fatalf("expected outer rollback to discard row, got %#v", outerRolledBack)
	}

	// The committed row should be visible in the default policy context.
	var committed []Product
	if err := db.WithContext(baseCtx).Find(&committed, orm.Where("sku = ?", "TX-COMMIT")); err != nil {
		t.Fatalf("find committed row: %v", err)
	}
	if len(committed) != 1 {
		t.Fatalf("expected one committed row, got %#v", committed)
	}
	if committed[0].CompanyID != "company-a" || committed[0].CreatedBy != "user-a" || committed[0].UpdatedBy != "user-a" {
		t.Fatalf("expected company and audit fields to be populated, got %#v", committed[0])
	}

	var rolledBack []Product
	if err := db.WithContext(baseCtx).Find(&rolledBack, orm.Where("sku = ?", "TX-ROLLBACK")); err != nil {
		t.Fatalf("find rolled back row: %v", err)
	}
	if len(rolledBack) != 0 {
		t.Fatalf("expected nested rollback to discard row, got %#v", rolledBack)
	}

	if err := db.Transaction(access.WithContext(context.Background(), access.Context{
		CompanyID: "company-b",
		UserID:    "user-b",
	}), func(tx *orm.DB) error {
		return tx.Create(&Product{
			ID:         "tx-company-b",
			SKU:        "TX-COMPANY-B",
			Name:       "Company B",
			PriceCents: 333,
		})
	}); err != nil {
		t.Fatalf("create company b row: %v", err)
	}

	var defaultRows []Product
	if err := db.WithContext(baseCtx).Find(&defaultRows, orm.OrderBy("sku ASC")); err != nil {
		t.Fatalf("default policy find: %v", err)
	}
	if got := productSKUs(defaultRows); len(got) != 1 || got[0] != "TX-COMMIT" {
		t.Fatalf("expected default policy to see only company-a row, got %v", got)
	}

	var ignoreCompanyRows []Product
	if err := db.WithPolicy(access.IgnoreCompany()).WithContext(baseCtx).Find(&ignoreCompanyRows, orm.OrderBy("sku ASC")); err != nil {
		t.Fatalf("ignore company find: %v", err)
	}
	if got := productSKUs(ignoreCompanyRows); !containsAll(got, []string{"TX-COMMIT", "TX-COMPANY-B"}) {
		t.Fatalf("expected ignore company to see both rows, got %v", got)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	var sawBegin, sawCommit, sawRollback bool
	for _, span := range provider.spans {
		switch span.Name {
		case "db.begin":
			sawBegin = true
		case "db.commit":
			sawCommit = true
		case "db.rollback":
			sawRollback = true
		}
	}
	if !sawBegin || !sawCommit || !sawRollback {
		t.Fatalf("expected transaction spans, got %#v", provider.spans)
	}
	if !strings.Contains(strings.Join(productSKUs(defaultRows), ","), "TX-COMMIT") {
		t.Fatalf("expected committed SKU in result set, got %#v", defaultRows)
	}
}

func productSKUs(rows []Product) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.SKU)
	}
	return out
}

func containsAll(values, expected []string) bool {
	have := map[string]struct{}{}
	for _, value := range values {
		have[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := have[value]; !ok {
			return false
		}
	}
	return true
}
