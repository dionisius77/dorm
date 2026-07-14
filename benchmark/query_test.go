package benchmark_test

import (
	"testing"

	"github.com/dionisius77/dorm/benchmark/testutil"
	postgresdialect "github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/orm"
)

func BenchmarkQueryGeneration(b *testing.B) {
	b.ReportAllocs()
	d := postgresdialect.New()
	columns := []string{
		`"id"`,
		`"company_id"`,
		`"email"`,
		`"name"`,
	}
	where := []string{
		`"company_id" = $1`,
		`"deleted_at" IS NULL`,
	}
	orderBy := []string{`"name" ASC`}
	limit := 25
	offset := 10

	b.Run("Select", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := d.RenderSelect("users", columns, where, orderBy, &limit, &offset); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Insert", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := d.RenderInsert("users", []string{`"id"`, `"company_id"`, `"email"`}, []string{`"id"`}); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Update", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := d.RenderUpdate("users", []string{`"name" = $1`}, []string{`"id" = $2`}, []string{`"id"`}); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Delete", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := d.RenderDelete("users", []string{`"id" = $1`}, nil); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkQueryExecution(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := benchmarkContext("bench-user", "bench-company", "bench-workspace")
	session := db.WithContext(ctx)
	seedUsers(b, session, "bench-query", 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var users []testutil.User
		if err := session.Find(&users, orm.OrderBy("email ASC"), orm.Limit(25)); err != nil {
			b.Fatal(err)
		}
	}
}
