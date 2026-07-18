package benchmark_test

import (
	"context"
	"testing"

	"github.com/dionisius77/dorm/benchmark/testutil"
	driverpostgres "github.com/dionisius77/dorm/driver/postgres"
	"github.com/dionisius77/dorm/schema"
)

func BenchmarkSchemaDrift(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	fixture.Project.SaveSnapshot(b, fixture.Schema)

	db := fixture.Project.OpenSQL(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		report, err := schema.DetectDriftFromSource(context.Background(), fixture.Project.ModelsDir, driverpostgres.New(driverpostgres.Config{DSN: fixture.Project.DSN, SchemaName: fixture.Project.Schema}).Inspector(), db, fixture.Project.Schema, fixture.Project.SnapshotPath)
		if err != nil {
			b.Fatal(err)
		}
		if report == nil {
			b.Fatal("expected drift report")
		}
	}
}
