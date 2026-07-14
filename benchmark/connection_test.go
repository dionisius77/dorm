package benchmark_test

import (
	"context"
	"testing"

	"github.com/dionisius77/dorm"
	"github.com/dionisius77/dorm/benchmark/testutil"
	driverpg "github.com/dionisius77/dorm/driver/postgres"
)

func BenchmarkOpen(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	ctx := context.Background()
	cfg := driverpg.Config{DSN: fixture.Project.DSN}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db, err := dorm.Open(ctx, driverpg.New(cfg))
		if err != nil {
			b.Fatal(err)
		}
		_ = db.Close()
	}
}

func BenchmarkPing(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	ctx := context.Background()
	db, err := dorm.Open(ctx, driverpg.New(driverpg.Config{DSN: fixture.Project.DSN}))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.Ping(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
