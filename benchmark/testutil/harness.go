package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dionisius77/dorm"
	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/internal/itest"
	"github.com/dionisius77/dorm/orm"
	"github.com/dionisius77/dorm/schema"
)

type Fixture struct {
	Project *itest.Project
	Schema  *schema.Schema
}

func NewFixture(t testing.TB, source string) *Fixture {
	t.Helper()
	project := itest.NewProject(t)
	WriteModels(t, project, source)
	s := BuildSchema(t, project)
	ApplySchema(t, project, s)
	return &Fixture{Project: project, Schema: s}
}

func WriteModels(t testing.TB, project *itest.Project, source string) {
	t.Helper()
	project.WriteFile(t, filepath.Join("models", "models.go"), source)
}

func BuildSchema(t testing.TB, project *itest.Project) *schema.Schema {
	t.Helper()
	s, err := schema.NewBuilder(project.ModelsDir).Build(context.Background())
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	return s
}

func ApplySchema(t testing.TB, project *itest.Project, s *schema.Schema) {
	t.Helper()
	if s == nil {
		t.Fatal("nil schema")
	}
	diff, err := schema.Compare(s, &schema.Schema{Name: s.Name})
	if err != nil {
		t.Fatalf("compare schema: %v", err)
	}
	sqls, err := postgres.New().RenderMigration(diff)
	if err != nil {
		t.Fatalf("render migration: %v", err)
	}
	db := project.OpenSQL(t)
	defer db.Close()
	for _, stmt := range sqls {
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("apply schema: %v\nsql: %s", err, stmt)
		}
	}
}

func OpenDB(t testing.TB, project *itest.Project, obs orm.ObservabilityConfig) *dorm.DB {
	t.Helper()
	db := project.OpenDB(t, obs, false)
	return db
}

func ExecSQL(t testing.TB, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), stmt, args...); err != nil {
		t.Fatalf("exec sql: %v\nsql: %s", err, stmt)
	}
}

func TracingConfig(enabled bool) orm.ObservabilityConfig {
	return orm.ObservabilityConfig{
		Tracing:  enabled,
		TraceSQL: orm.TraceSQLDisabled,
	}
}

func AccessContext(userID, companyID, workspaceID string) context.Context {
	return access.WithContext(context.Background(), access.Context{
		UserID:      userID,
		CompanyID:   companyID,
		WorkspaceID: workspaceID,
	})
}

func StringID(prefix string, n int) string {
	return fmt.Sprintf("%s-%06d", prefix, n)
}
