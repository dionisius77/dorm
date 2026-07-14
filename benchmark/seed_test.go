package benchmark_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dionisius77/dorm/benchmark/testutil"
	"github.com/dionisius77/dorm/seed"
)

func BenchmarkSeedSync(b *testing.B) {
	b.ReportAllocs()
	fixture := testutil.NewFixture(b, testutil.SeedModelsSource)
	db := testutil.OpenDB(b, fixture.Project, testutil.TracingConfig(false))

	ctx := context.Background()

	b.Run("Single", func(b *testing.B) {
		roles := makeSeedRoles(1)
		seedRoles(b, db, ctx, roles)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := seed.Sync(ctx, db, roles, seed.Key("Code")); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("100Records", func(b *testing.B) {
		roles := makeSeedRoles(100)
		seedRoles(b, db, ctx, roles)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := seed.Sync(ctx, db, roles, seed.Key("Code")); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("1000Records", func(b *testing.B) {
		roles := makeSeedRoles(1000)
		seedRoles(b, db, ctx, roles)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := seed.Sync(ctx, db, roles, seed.Key("Code")); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func makeSeedRoles(count int) []testutil.Role {
	roles := make([]testutil.Role, count)
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		roles[i] = testutil.Role{
			ID:        testutil.StringID("bench-role", i),
			Code:      fmt.Sprintf("ROLE_%04d", i),
			Name:      fmt.Sprintf("Role %03d", i),
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	return roles
}

func seedRoles(b *testing.B, db seed.SessionProvider, ctx context.Context, roles []testutil.Role) {
	b.Helper()
	if err := seed.Sync(ctx, db, roles, seed.Key("Code")); err != nil {
		b.Fatal(err)
	}
}
