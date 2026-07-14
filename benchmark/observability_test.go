package benchmark_test

import (
	"testing"

	"github.com/dionisius77/dorm/benchmark/testutil"
	"github.com/dionisius77/dorm/orm"
)

func BenchmarkObservability(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)

	ctx := benchmarkContext("bench-user", "bench-company", "bench-workspace")
	seedSession := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false)).WithContext(ctx)
	seedUsers(b, seedSession, "bench-observability", 200)

	disabled := testutil.OpenDB(b, fixture.Project, orm.ObservabilityConfig{
		Tracing:  false,
		TraceSQL: orm.TraceSQLDisabled,
	})
	enabled := testutil.OpenDB(b, fixture.Project, orm.ObservabilityConfig{
		Tracing:        true,
		TraceSQL:       orm.TraceSQLDisabled,
		TracerProvider: testutil.NopTracerProvider{},
	})

	b.Run("TracingDisabled", func(b *testing.B) {
		runObservedFind(b, disabled.WithContext(ctx))
	})
	b.Run("TracingEnabled", func(b *testing.B) {
		runObservedFind(b, enabled.WithContext(ctx))
	})
}

func runObservedFind(b *testing.B, session *orm.Session) {
	b.Helper()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var users []testutil.User
		if err := session.Find(&users, orm.OrderBy("email ASC"), orm.Limit(25)); err != nil {
			b.Fatal(err)
		}
	}
}
