package benchmark_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dionisius77/dorm/benchmark/testutil"
	"github.com/dionisius77/dorm/orm"
)

func BenchmarkCommit(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.Tx(ctx, func(tx *orm.Session) error {
			_ = tx
			return nil
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRollback(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.Tx(ctx, func(tx *orm.Session) error {
			_ = tx
			return errors.New("rollback")
		}); err == nil {
			b.Fatal("expected rollback error")
		}
	}
}
