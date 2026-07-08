package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dionisius77/dorm/dialect"
	"github.com/dionisius77/dorm/schema"
)

type Config struct {
	Root          string
	MigrationsDir string
	SnapshotPath  string
	SchemaName    string
	Dialect       dialect.Dialect
	Inspector     schema.Inspector
}

type Service struct {
	Config Config
}

type Result struct {
	MigrationName string
	UpSQL         []string
	DownSQL       []string
	Diff          *schema.Diff
	Snapshot      *schema.Snapshot
}

func NewService(cfg Config) *Service {
	return &Service{Config: cfg}
}

func (s *Service) Generate(ctx context.Context) (*Result, error) {
	if s == nil {
		return nil, fmt.Errorf("migrate: nil service")
	}
	if s.Config.Dialect == nil {
		return nil, fmt.Errorf("migrate: nil dialect")
	}
	builder := schema.NewBuilder(s.Config.Root)
	current, err := builder.Build(ctx)
	if err != nil {
		return nil, err
	}
	var previous *schema.Schema
	if s.Config.SnapshotPath != "" {
		if snap, err := schema.LoadSnapshot(s.Config.SnapshotPath); err == nil && snap != nil {
			previous = snap.Schema
		}
	}
	if previous == nil {
		previous = &schema.Schema{Name: current.Name}
	}
	diff, err := schema.Compare(current, previous)
	if err != nil {
		return nil, err
	}
	if diff.Empty() {
		return &Result{Diff: diff, Snapshot: &schema.Snapshot{GeneratedAt: time.Now().UTC(), Schema: current}}, nil
	}
	upSQL, err := s.Config.Dialect.RenderMigration(diff)
	if err != nil {
		return nil, err
	}
	downDiff := invertDiff(diff)
	downSQL, err := s.Config.Dialect.RenderMigration(downDiff)
	if err != nil {
		return nil, err
	}
	name := s.nextMigrationName()
	snapshot := &schema.Snapshot{GeneratedAt: time.Now().UTC(), Schema: current}
	return &Result{
		MigrationName: name,
		UpSQL:         upSQL,
		DownSQL:       downSQL,
		Diff:          diff,
		Snapshot:      snapshot,
	}, nil
}

func (s *Service) Write(result *Result) error {
	if s == nil {
		return fmt.Errorf("migrate: nil service")
	}
	if result == nil {
		return fmt.Errorf("migrate: nil result")
	}
	if s.Config.MigrationsDir == "" {
		return fmt.Errorf("migrate: empty migrations dir")
	}
	if err := os.MkdirAll(s.Config.MigrationsDir, 0o755); err != nil {
		return err
	}
	if result.MigrationName == "" {
		result.MigrationName = s.nextMigrationName()
	}
	upPath := filepath.Join(s.Config.MigrationsDir, result.MigrationName+".up.sql")
	downPath := filepath.Join(s.Config.MigrationsDir, result.MigrationName+".down.sql")
	if err := os.WriteFile(upPath, []byte(strings.Join(result.UpSQL, "\n\n")+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(downPath, []byte(strings.Join(result.DownSQL, "\n\n")+"\n"), 0o644); err != nil {
		return err
	}
	if s.Config.SnapshotPath != "" && result.Snapshot != nil {
		if err := schema.SaveSnapshot(s.Config.SnapshotPath, result.Snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Run(ctx context.Context, db *sql.DB) error {
	if s == nil {
		return fmt.Errorf("migrate: nil service")
	}
	if db == nil {
		return fmt.Errorf("migrate: nil db")
	}
	if s.Config.MigrationsDir == "" {
		return fmt.Errorf("migrate: empty migrations dir")
	}
	if err := ensureMigrationRegistry(ctx, db); err != nil {
		return err
	}
	applied, err := appliedMigrationSet(ctx, db)
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(s.Config.MigrationsDir, "*.up.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".up.sql")
		if applied[name] {
			continue
		}
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", filepath.Base(file), err)
		}
		if err := recordAppliedMigration(ctx, db, name, checksum(sqlBytes)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Revert(ctx context.Context, db *sql.DB, name string) error {
	if s == nil {
		return fmt.Errorf("migrate: nil service")
	}
	if db == nil {
		return fmt.Errorf("migrate: nil db")
	}
	if name == "" {
		return fmt.Errorf("migrate: empty migration name")
	}
	path := filepath.Join(s.Config.MigrationsDir, name+".down.sql")
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, string(sqlBytes)); err != nil {
		return err
	}
	return removeAppliedMigration(ctx, db, name)
}

func (s *Service) Status() ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("migrate: nil service")
	}
	files, err := filepath.Glob(filepath.Join(s.Config.MigrationsDir, "*.up.sql"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var names []string
	for _, file := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(file), ".up.sql"))
	}
	return names, nil
}

func (s *Service) nextMigrationName() string {
	files, _ := filepath.Glob(filepath.Join(s.Config.MigrationsDir, "*.up.sql"))
	n := len(files) + 1
	return fmt.Sprintf("%04d_schema", n)
}

func invertDiff(diff *schema.Diff) *schema.Diff {
	if diff == nil {
		return nil
	}
	out := &schema.Diff{Operations: make([]schema.Operation, 0, len(diff.Operations))}
	for i := len(diff.Operations) - 1; i >= 0; i-- {
		op := diff.Operations[i]
		switch op.Kind {
		case schema.OpCreateTable:
			out.Operations = append(out.Operations, schema.Operation{Kind: schema.OpDropTable, Table: op.Table})
		case schema.OpDropTable:
			// Cannot safely reconstruct a dropped table without snapshot history.
		case schema.OpAddColumn:
			out.Operations = append(out.Operations, schema.Operation{Kind: schema.OpDropColumn, Table: op.Table, Column: op.Column})
		case schema.OpDropColumn:
			out.Operations = append(out.Operations, schema.Operation{Kind: schema.OpAddColumn, Table: op.Table, Column: op.Column})
		case schema.OpAlterColumn:
			out.Operations = append(out.Operations, schema.Operation{Kind: schema.OpAlterColumn, Table: op.Table, Column: op.Previous, Previous: op.Column})
		case schema.OpCreateIndex:
			out.Operations = append(out.Operations, schema.Operation{Kind: schema.OpDropIndex, Table: op.Table, Index: op.Index})
		case schema.OpDropIndex:
			out.Operations = append(out.Operations, schema.Operation{Kind: schema.OpCreateIndex, Table: op.Table, Index: op.Index})
		case schema.OpCreateConstraint:
			out.Operations = append(out.Operations, schema.Operation{Kind: schema.OpDropConstraint, Table: op.Table, Constraint: op.Constraint})
		case schema.OpDropConstraint:
			out.Operations = append(out.Operations, schema.Operation{Kind: schema.OpCreateConstraint, Table: op.Table, Constraint: op.Constraint})
		}
	}
	return out
}

func ensureMigrationRegistry(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS orm_migrations (
			name text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now(),
			checksum text NOT NULL
		)
	`)
	return err
}

func appliedMigrationSet(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM orm_migrations ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

func recordAppliedMigration(ctx context.Context, db *sql.DB, name, checksum string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO orm_migrations (name, checksum) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING`, name, checksum)
	return err
}

func removeAppliedMigration(ctx context.Context, db *sql.DB, name string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM orm_migrations WHERE name = $1`, name)
	return err
}

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
