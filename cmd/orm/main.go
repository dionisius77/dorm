package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dionisius77/dorm"
	"github.com/dionisius77/dorm/dialect/postgres"
	driverpostgres "github.com/dionisius77/dorm/driver/postgres"
	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/migrate"
	"github.com/dionisius77/dorm/schema"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "init":
		return cmdInit(args[1:])
	case "migrate":
		return cmdMigrate(ctx, args[1:])
	case "schema":
		return cmdSchema(ctx, args[1:])
	case "doctor":
		return cmdDoctor(ctx, args[1:])
	default:
		return usage()
	}
}

func usage() error {
	fmt.Fprintln(os.Stderr, "usage: orm <init|migrate|schema|doctor>")
	return nil
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := defaultConfig()
	cfg.Root = *root
	if err := os.MkdirAll(filepath.Join(*root, cfg.MigrationsDir), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(*root, cfg.SchemasDir), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(*root, cfg.ModelsDir), 0o755); err != nil {
		return err
	}
	return saveConfig(filepath.Join(*root, cfg.ConfigFile), cfg)
}

func cmdMigrate(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "generate":
		return cmdMigrateGenerate(ctx, args[1:])
	case "run":
		return cmdMigrateRun(ctx, args[1:])
	case "revert":
		return cmdMigrateRevert(ctx, args[1:])
	case "status":
		return cmdMigrateStatus(ctx, args[1:])
	default:
		return usage()
	}
}

func cmdMigrateGenerate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate generate", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return err
	}
	service := migrate.NewService(migrate.Config{
		Root:          filepath.Join(*root, cfg.ModelsDir),
		MigrationsDir: filepath.Join(*root, cfg.MigrationsDir),
		SnapshotPath:  filepath.Join(*root, cfg.SnapshotPath),
		SchemaName:    cfg.SchemaName,
		Dialect:       postgres.New(),
		Inspector:     schema.PostgresInspector{},
	})
	result, err := service.Generate(ctx)
	if err != nil {
		return err
	}
	return service.Write(result)
}

func cmdMigrateRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate run", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return err
	}
	db, err := openConfiguredDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	service := migrate.NewService(migrate.Config{
		Root:          filepath.Join(*root, cfg.ModelsDir),
		MigrationsDir: filepath.Join(*root, cfg.MigrationsDir),
		SnapshotPath:  filepath.Join(*root, cfg.SnapshotPath),
		SchemaName:    cfg.SchemaName,
		Dialect:       postgres.New(),
		Inspector:     schema.PostgresInspector{},
	})
	return service.Run(ctx, db.SQLDB())
}

func cmdMigrateRevert(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate revert", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	name := fs.String("name", "", "migration name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errkind.New(errkind.KindConfiguration, "migrate revert requires --name")
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return err
	}
	db, err := openConfiguredDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	service := migrate.NewService(migrate.Config{
		Root:          filepath.Join(*root, cfg.ModelsDir),
		MigrationsDir: filepath.Join(*root, cfg.MigrationsDir),
		SnapshotPath:  filepath.Join(*root, cfg.SnapshotPath),
		SchemaName:    cfg.SchemaName,
		Dialect:       postgres.New(),
		Inspector:     schema.PostgresInspector{},
	})
	return service.Revert(ctx, db.SQLDB(), *name)
}

func cmdMigrateStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate status", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return err
	}
	db, err := openConfiguredDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(context.Background()); err != nil {
		return err
	}
	if err := ensureMigrationRegistry(context.Background(), db.SQLDB()); err != nil {
		return err
	}
	applied, err := appliedMigrationSet(context.Background(), db.SQLDB())
	if err != nil {
		return err
	}
	service := migrate.NewService(migrate.Config{
		Root:          filepath.Join(*root, cfg.ModelsDir),
		MigrationsDir: filepath.Join(*root, cfg.MigrationsDir),
		SnapshotPath:  filepath.Join(*root, cfg.SnapshotPath),
		SchemaName:    cfg.SchemaName,
		Dialect:       postgres.New(),
		Inspector:     schema.PostgresInspector{},
	})
	names, err := service.Status()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "applied:")
	for _, name := range names {
		if applied[name] {
			fmt.Fprintf(os.Stdout, "- %s\n", name)
		}
	}
	fmt.Fprintln(os.Stdout, "pending:")
	for _, name := range names {
		if !applied[name] {
			fmt.Fprintf(os.Stdout, "- %s\n", name)
		}
	}
	return nil
}

func cmdSchema(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "check":
		return cmdSchemaCheck(ctx, args[1:])
	default:
		return usage()
	}
}

func cmdSchemaCheck(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("schema check", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return err
	}
	db, err := openConfiguredDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	report, err := schema.DetectDriftFromSource(
		ctx,
		filepath.Join(*root, cfg.ModelsDir),
		schema.PostgresInspector{},
		db.SQLDB(),
		cfg.SchemaName,
		filepath.Join(*root, cfg.SnapshotPath),
	)
	if err != nil {
		return err
	}
	if report != nil && !report.Clean() {
		fmt.Fprintln(os.Stdout, "Schema Drift Detected")
		if report.ExpectedDiff != nil {
			for _, op := range report.ExpectedDiff.Operations {
				fmt.Fprintf(os.Stdout, "- %s %s\n", op.Kind, op.Table)
			}
		}
		if report.HasSnapshotDrift() {
			fmt.Fprintln(os.Stdout, "Snapshot Drift Detected")
			for _, op := range report.SnapshotDiff.Operations {
				fmt.Fprintf(os.Stdout, "- %s %s\n", op.Kind, op.Table)
			}
		}
		return errkind.New(errkind.KindInvalidSchema, "schema drift detected")
	}
	fmt.Fprintln(os.Stdout, "Schema OK")
	return nil
}

func cmdDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := dorm.DefaultCompatibilityPolicy().ValidateRuntime(); err != nil {
		return errkind.Wrap(errkind.KindUnsupportedFeature, "doctor: compatibility check failed", err)
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return err
	}
	if cfg.Root == "" || cfg.MigrationsDir == "" || cfg.SnapshotPath == "" {
		return errkind.New(errkind.KindConfiguration, "doctor: incomplete config")
	}
	if cfg.Driver != postgres.New().Name() {
		return errkind.New(errkind.KindUnsupportedFeature, fmt.Sprintf("doctor: unsupported driver %q", cfg.Driver))
	}
	snapshot, err := schema.LoadSnapshot(filepath.Join(*root, cfg.SnapshotPath))
	if err != nil {
		return errkind.Wrap(errkind.KindInvalidSchema, "doctor: snapshot integrity check failed", err)
	}
	if snapshot == nil || snapshot.Schema == nil {
		return errkind.New(errkind.KindInvalidSchema, "doctor: snapshot integrity check failed: empty snapshot")
	}
	if err := snapshot.Schema.Validate(); err != nil {
		return errkind.Wrap(errkind.KindInvalidSchema, "doctor: snapshot integrity check failed", err)
	}
	db, err := openConfiguredDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return errkind.Wrap(errkind.KindRuntimeQuery, "doctor: connectivity check failed", err)
	}
	fmt.Println("doctor: compatibility ok")
	fmt.Println("doctor: config ok")
	fmt.Println("doctor: connectivity ok")
	fmt.Println("doctor: snapshot ok")
	fmt.Println("doctor: dialect ok")
	return nil
}

type cliConfig struct {
	Root          string `json:"root"`
	ModelsDir     string `json:"models_dir"`
	MigrationsDir string `json:"migrations_dir"`
	SchemasDir    string `json:"schemas_dir"`
	SnapshotPath  string `json:"snapshot_path"`
	SchemaName    string `json:"schema_name"`
	Driver        string `json:"driver"`
	DSN           string `json:"dsn"`
	ConfigFile    string `json:"-"`
}

func defaultConfig() cliConfig {
	return cliConfig{
		Root:          ".",
		ModelsDir:     "models",
		MigrationsDir: "migrations",
		SchemasDir:    "schemas",
		SnapshotPath:  filepath.Join("schemas", "current.snapshot.json"),
		SchemaName:    "public",
		Driver:        "postgres",
		DSN:           "",
		ConfigFile:    "orm.json",
	}
}

func saveConfig(path string, cfg cliConfig) error {
	cfg.ConfigFile = ""
	data, err := jsonMarshalIndent(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func loadConfig(path string) (cliConfig, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := jsonUnmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Kept as small wrappers so the CLI file remains easy to scan.
func jsonMarshalIndent(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func openConfiguredDB(ctx context.Context, cfg cliConfig) (*dorm.DB, error) {
	driver := driverpostgres.New(driverpostgres.Config{
		DSN:        cfg.DSN,
		DriverName: cfg.Driver,
	})
	dorm.RegisterDriver(driver)
	return dorm.Open(ctx, driver)
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
