package benchmark_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/benchmark/testutil"
	"github.com/dionisius77/dorm/orm"
)

func BenchmarkCreate(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := benchmarkContext("bench-user", "bench-company", "bench-workspace")
	session := db.WithContext(ctx)
	now := time.Now().UTC()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := testutil.User{
			ID:        testutil.StringID("bench-create", i),
			CompanyID: "bench-company",
			Email:     fmt.Sprintf("create-%d@example.com", i),
			Name:      "Create Bench",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := session.Create(&user); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	testutil.ExecSQL(b, db.SQLDB(), `DELETE FROM users WHERE id LIKE 'bench-create-%'`)
}

func BenchmarkFindByID(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := benchmarkContext("bench-user", "bench-company", "bench-workspace")
	session := db.WithContext(ctx)
	seedUser(b, session, "bench-find-id", "find@example.com", "Find One")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var users []testutil.User
		if err := session.FindOne(&users, orm.Where("id = ?", "bench-find-id")); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFindMany(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := benchmarkContext("bench-user", "bench-company", "bench-workspace")
	session := db.WithContext(ctx)
	seedUsers(b, session, "bench-find-many", 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var users []testutil.User
		if err := session.Find(&users, orm.OrderBy("email ASC")); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpdate(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := benchmarkContext("bench-user", "bench-company", "bench-workspace")
	session := db.WithContext(ctx)
	seedUser(b, session, "bench-update", "update@example.com", "Original")

	updated := testutil.User{
		ID:        "bench-update",
		CompanyID: "bench-company",
		Email:     "update@example.com",
		Name:      "Updated",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := session.Update(&updated); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDelete(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.AppModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := benchmarkContext("bench-user", "bench-company", "bench-workspace")
	session := db.WithContext(ctx)
	seedUsers(b, session, "bench-delete", b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := testutil.User{ID: testutil.StringID("bench-delete", i)}
		if err := session.Delete(&user); err != nil {
			b.Fatal(err)
		}
	}
}

func seedUser(b *testing.B, session *orm.Session, id, email, name string) {
	b.Helper()
	now := time.Now().UTC()
	user := testutil.User{
		ID:        id,
		CompanyID: "bench-company",
		Email:     email,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := session.Create(&user); err != nil {
		b.Fatal(err)
	}
}

func seedUsers(b *testing.B, session *orm.Session, prefix string, count int) {
	b.Helper()
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		user := testutil.User{
			ID:        testutil.StringID(prefix, i),
			CompanyID: "bench-company",
			Email:     fmt.Sprintf("%s-%d@example.com", prefix, i),
			Name:      strings.Title(prefix),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := session.Create(&user); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkContext(userID, companyID, workspaceID string) context.Context {
	return access.WithContext(context.Background(), access.Context{
		UserID:      userID,
		CompanyID:   companyID,
		WorkspaceID: workspaceID,
	})
}
