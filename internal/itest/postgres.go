package itest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dionisius77/dorm"
	"github.com/dionisius77/dorm/driver/postgres"
	"github.com/dionisius77/dorm/orm"
	"github.com/dionisius77/dorm/schema"
)

// Project owns an isolated PostgreSQL-backed test workspace.
type Project struct {
	BaseDSN       string
	Schema        string
	Root          string
	ModelsDir     string
	MigrationsDir string
	SchemasDir    string
	SnapshotPath  string
	ConfigPath    string
	DSN           string
}

// DefaultModelSource is a compact schema fixture used across integration tests.
const DefaultModelSource = `package models

import (
	"time"
)

type Product struct {
	ID         string     ` + "`orm:\"pk\"`" + `
	CompanyID  string     ` + "`orm:\"company\"`" + `
	SKU        string     ` + "`orm:\"unique\"`" + `
	Name       string
	PriceCents int64
	DeletedAt  *time.Time ` + "`orm:\"soft_delete\"`" + `
	CreatedAt  time.Time  ` + "`orm:\"created_at\"`" + `
	CreatedBy  string     ` + "`orm:\"created_by\"`" + `
	UpdatedAt  time.Time  ` + "`orm:\"updated_at\"`" + `
	UpdatedBy  string     ` + "`orm:\"updated_by\"`" + `
	DeletedBy  string     ` + "`orm:\"deleted_by\"`" + `
}

type Role struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	Code      string    ` + "`orm:\"unique\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
}
`

// NewProject creates a temporary workspace and schema for integration tests.
func NewProject(t testing.TB) *Project {
	t.Helper()
	baseDSN := RequireDSN(t)
	root := t.TempDir()
	schemaName := fmt.Sprintf("dorm_it_%d", time.Now().UTC().UnixNano())
	p := &Project{
		BaseDSN:       baseDSN,
		Schema:        schemaName,
		Root:          root,
		ModelsDir:     filepath.Join(root, "models"),
		MigrationsDir: filepath.Join(root, "migrations"),
		SchemasDir:    filepath.Join(root, "schemas"),
		SnapshotPath:  filepath.Join(root, "schemas", "current.snapshot.json"),
		ConfigPath:    filepath.Join(root, "orm.json"),
		DSN:           WithSearchPath(baseDSN, schemaName),
	}
	for _, dir := range []string{p.ModelsDir, p.MigrationsDir, p.SchemasDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create workspace dir %s: %v", dir, err)
		}
	}
	CreateSchema(t, baseDSN, schemaName)
	t.Cleanup(func() {
		DropSchema(t, baseDSN, schemaName)
	})
	return p
}

// RequireDSN returns the PostgreSQL DSN used by the integration tests.
func RequireDSN(t testing.TB) string {
	t.Helper()
	for _, key := range []string{"DORM_TEST_POSTGRES_DSN", "DATABASE_URL", "POSTGRES_DSN"} {
		if dsn := strings.TrimSpace(os.Getenv(key)); dsn != "" {
			return dsn
		}
	}
	t.Skip("set DORM_TEST_POSTGRES_DSN, DATABASE_URL, or POSTGRES_DSN to run PostgreSQL integration tests")
	return ""
}

// WithSearchPath returns a DSN that targets the given PostgreSQL schema.
func WithSearchPath(dsn, schemaName string) string {
	dsn = strings.TrimSpace(dsn)
	schemaName = strings.TrimSpace(schemaName)
	if dsn == "" || schemaName == "" {
		return dsn
	}
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" && u.Host != "" {
		q := u.Query()
		q.Set("search_path", schemaName)
		u.RawQuery = q.Encode()
		return u.String()
	}
	if strings.Contains(strings.ToLower(dsn), "search_path=") {
		return replaceKeywordParam(dsn, "search_path", schemaName)
	}
	return dsn + " search_path=" + schemaName
}

// CreateSchema creates a PostgreSQL schema if it does not already exist.
func CreateSchema(t testing.TB, dsn, schemaName string) {
	t.Helper()
	db := openAdminDB(t, dsn)
	defer db.Close()
	stmt := fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, quoteIdent(schemaName))
	if _, err := db.ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("create schema %s: %v", schemaName, err)
	}
}

// DropSchema removes a PostgreSQL schema and its contents.
func DropSchema(t testing.TB, dsn, schemaName string) {
	t.Helper()
	db := openAdminDB(t, dsn)
	defer db.Close()
	stmt := fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, quoteIdent(schemaName))
	if _, err := db.ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("drop schema %s: %v", schemaName, err)
	}
}

// WriteFile writes a project file relative to the workspace root.
func (p *Project) WriteFile(t testing.TB, relPath, contents string) {
	t.Helper()
	path := filepath.Join(p.Root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create file dir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

// WriteJSON writes a project file as pretty-printed JSON.
func (p *Project) WriteJSON(t testing.TB, relPath string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	p.WriteFile(t, relPath, string(data))
}

// BuildSchema parses the workspace models into an in-memory schema.
func (p *Project) BuildSchema(t testing.TB) *schema.Schema {
	t.Helper()
	s, err := schema.NewBuilder(p.ModelsDir).Build(context.Background())
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	return s
}

// SaveSnapshot persists a schema snapshot for later drift checks.
func (p *Project) SaveSnapshot(t testing.TB, s *schema.Schema) {
	t.Helper()
	if s == nil {
		t.Fatal("nil schema")
	}
	if err := schema.SaveSnapshot(p.SnapshotPath, &schema.Snapshot{Schema: s}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
}

// OpenDB opens a dorm DB using the project DSN.
func (p *Project) OpenDB(t testing.TB, obs orm.ObservabilityConfig, preflight bool) *dorm.DB {
	t.Helper()
	driver := postgres.New(postgres.Config{
		DSN:              p.DSN,
		PreflightEnabled: preflight,
		ModelRoot:        p.ModelsDir,
		SnapshotPath:     p.SnapshotPath,
		SchemaName:       p.Schema,
	})
	db, err := dorm.Open(context.Background(), driver, dorm.WithObservability(obs))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

// OpenSQL opens a raw *sql.DB against the project schema.
func (p *Project) OpenSQL(t testing.TB) *sql.DB {
	t.Helper()
	db, err := postgres.New(postgres.Config{DSN: p.DSN}).Open(context.Background())
	if err != nil {
		t.Fatalf("open sql db: %v", err)
	}
	return db
}

func openAdminDB(t testing.TB, dsn string) *sql.DB {
	t.Helper()
	db, err := postgres.New(postgres.Config{DSN: dsn}).Open(context.Background())
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	return db
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func replaceKeywordParam(dsn, key, value string) string {
	parts := strings.Fields(dsn)
	out := make([]string, 0, len(parts)+1)
	found := false
	for _, part := range parts {
		if strings.HasPrefix(strings.ToLower(part), strings.ToLower(key)+"=") {
			out = append(out, key+"="+value)
			found = true
			continue
		}
		out = append(out, part)
	}
	if !found {
		out = append(out, key+"="+value)
	}
	return strings.Join(out, " ")
}
