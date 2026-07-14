package benchmark_test

import (
	"context"
	"testing"

	"github.com/dionisius77/dorm/benchmark/testutil"
	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/migrate"
	"github.com/dionisius77/dorm/schema"
)

func BenchmarkMigrationGenerate(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	testutil.WriteModels(b, fixture.Project, testutil.MigrationModelsSourceV2)
	fixture.Project.SaveSnapshot(b, fixture.Schema)

	service := migrate.NewService(migrate.Config{
		Root:         fixture.Project.ModelsDir,
		SnapshotPath: fixture.Project.SnapshotPath,
		Dialect:      postgres.New(),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.Generate(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMigrationSchemaComparison(b *testing.B) {
	b.ReportAllocs()
	oldFixture := testutil.NewFixture(b, testutil.AppModelsSource)
	newFixture := testutil.NewFixture(b, testutil.MigrationModelsSourceV2)
	oldSchema := oldFixture.Schema
	newSchema := newFixture.Schema

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := schema.Compare(newSchema, oldSchema); err != nil {
			b.Fatal(err)
		}
	}
}
