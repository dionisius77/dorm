package benchmark_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/dionisius77/dorm/benchmark/testutil"
)

func BenchmarkAudit(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := benchmarkContext("audit-user", "audit-company", "audit-workspace")
	session := db.WithContext(ctx)

	b.Run("Create", func(b *testing.B) {
		now := time.Now().UTC()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			record := testutil.AuditRecord{
				ID:        testutil.StringID("bench-audit-create", i),
				CompanyID: "audit-company",
				Name:      fmt.Sprintf("Audit %d", i),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := session.Create(&record); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		testutil.ExecSQL(b, db.SQLDB(), `DELETE FROM audit_records WHERE id LIKE 'bench-audit-create-%'`)
	})

	b.Run("Update", func(b *testing.B) {
		record := testutil.AuditRecord{
			ID:        "bench-audit-update",
			CompanyID: "audit-company",
			Name:      "Original",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := session.Create(&record); err != nil {
			b.Fatal(err)
		}
		record.Name = "Updated"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := session.Update(&record); err != nil {
				b.Fatal(err)
			}
		}
	})
}
