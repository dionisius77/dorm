package benchmark_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/benchmark/testutil"
	"github.com/dionisius77/dorm/orm"
)

func BenchmarkAccessPolicy(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := benchmarkContext("bench-user", "company-a", "workspace-a")
	session := db.WithContext(ctx)
	seedProducts(b, session, "company-a", "workspace-a", "company-b", "workspace-b", 100)

	deleted := testutil.Product{
		ID:          "bench-product-deleted",
		CompanyID:   "company-a",
		WorkspaceID: "workspace-a",
		SKU:         "bench-product-deleted",
		Name:        "Deleted Product",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := session.Create(&deleted); err != nil {
		b.Fatal(err)
	}
	if err := session.Delete(&deleted); err != nil {
		b.Fatal(err)
	}

	b.Run("Default", func(b *testing.B) {
		runAccessFind(b, db.WithContext(ctx))
	})
	b.Run("IgnoreCompany", func(b *testing.B) {
		runAccessFind(b, db.WithPolicy(access.IgnoreCompany()).WithContext(ctx))
	})
	b.Run("IgnoreRLS", func(b *testing.B) {
		runAccessFind(b, db.WithPolicy(access.IgnoreRLS()).WithContext(ctx))
	})
	b.Run("System", func(b *testing.B) {
		runAccessFind(b, db.WithPolicy(access.System()).WithContext(ctx))
	})
}

func runAccessFind(b *testing.B, session *orm.Session) {
	b.Helper()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var products []testutil.Product
		if err := session.Find(&products, orm.OrderBy("name ASC")); err != nil {
			b.Fatal(err)
		}
	}
}

func seedProducts(b *testing.B, session *orm.Session, companyA, workspaceA, companyB, workspaceB string, count int) {
	b.Helper()
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		product := testutil.Product{
			ID:          testutil.StringID("bench-product-a", i),
			CompanyID:   companyA,
			WorkspaceID: workspaceA,
			SKU:         fmt.Sprintf("sku-a-%d", i),
			Name:        fmt.Sprintf("Alpha %03d", i),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := session.Create(&product); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < count; i++ {
		product := testutil.Product{
			ID:          testutil.StringID("bench-product-b", i),
			CompanyID:   companyB,
			WorkspaceID: workspaceA,
			SKU:         fmt.Sprintf("sku-b-%d", i),
			Name:        fmt.Sprintf("Beta %03d", i),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := session.Create(&product); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < count; i++ {
		product := testutil.Product{
			ID:          testutil.StringID("bench-product-c", i),
			CompanyID:   companyA,
			WorkspaceID: workspaceB,
			SKU:         fmt.Sprintf("sku-c-%d", i),
			Name:        fmt.Sprintf("Gamma %03d", i),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := session.Create(&product); err != nil {
			b.Fatal(err)
		}
	}
}
