package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dionisius77/dorm"
	"github.com/dionisius77/dorm/analyzer"
	"github.com/dionisius77/dorm/dialect/postgres"
	driverpostgres "github.com/dionisius77/dorm/driver/postgres"
	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/migrate"
	"github.com/dionisius77/dorm/schema"
	"github.com/dionisius77/dorm/seed"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printRootHelp(os.Stdout)
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		printRootHelp(os.Stdout)
		return nil
	case "init":
		return cmdInit(args[1:])
	case "migrate":
		return cmdMigrate(ctx, args[1:])
	case "schema":
		return cmdSchema(ctx, args[1:])
	case "seed":
		return cmdSeed(ctx, args[1:])
	case "analyze":
		return cmdAnalyze(ctx, args[1:])
	case "dry-run":
		return cmdDryRun(args[1:])
	case "doctor":
		return cmdDoctor(ctx, args[1:])
	default:
		printRootHelp(os.Stderr)
		return errkind.New(errkind.KindConfiguration, fmt.Sprintf("orm: unknown command %q", args[0]))
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func cmdInit(args []string) error {
	fs := newFlagSet("init")
	root := fs.String("root", ".", "project root")
	fs.Usage = func() { printInitHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	cfg := defaultConfig()
	cfg.Root = *root
	if err := os.MkdirAll(filepath.Join(*root, cfg.MigrationsDir), 0o755); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "init: create migrations directory", err)
	}
	if err := os.MkdirAll(filepath.Join(*root, cfg.SchemasDir), 0o755); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "init: create schemas directory", err)
	}
	if err := os.MkdirAll(filepath.Join(*root, cfg.ModelsDir), 0o755); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "init: create models directory", err)
	}
	if err := saveConfig(filepath.Join(*root, cfg.ConfigFile), cfg); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "init: write config", err)
	}
	fmt.Fprintln(os.Stdout, "✓ Project initialized")
	return nil
}

func cmdMigrate(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printMigrateHelp(os.Stdout)
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		printMigrateHelp(os.Stdout)
		return nil
	case "generate":
		return cmdMigrateGenerate(ctx, args[1:])
	case "run":
		return cmdMigrateRun(ctx, args[1:])
	case "revert", "rollback":
		return cmdMigrateRevert(ctx, args[1:])
	case "status":
		return cmdMigrateStatus(ctx, args[1:])
	default:
		printMigrateHelp(os.Stderr)
		return errkind.New(errkind.KindConfiguration, fmt.Sprintf("migrate: unknown command %q", args[0]))
	}
}

func cmdMigrateGenerate(ctx context.Context, args []string) error {
	fs := newFlagSet("migrate generate")
	root := fs.String("root", ".", "project root")
	fs.Usage = func() { printMigrateGenerateHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "migrate generate: load config", err)
	}
	if err := validateProjectPaths(*root, cfg, true); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "migrate generate: validate project", err)
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
	if result == nil || result.Diff == nil || result.Diff.Empty() {
		fmt.Fprintln(os.Stdout, "✓ No schema changes detected.")
		return nil
	}
	return service.Write(result)
}

func cmdMigrateRun(ctx context.Context, args []string) error {
	fs := newFlagSet("migrate run")
	root := fs.String("root", ".", "project root")
	fs.Usage = func() { printMigrateRunHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "migrate run: load config", err)
	}
	if err := validateProjectPaths(*root, cfg, false); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "migrate run: validate project", err)
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
	if err := service.Run(ctx, db.SQLDB()); err != nil {
		return errkind.Wrap(errkind.KindMigrationApplication, "migrate run: apply migrations", err)
	}
	fmt.Fprintln(os.Stdout, "✓ Migration completed")
	return nil
}

func cmdMigrateRevert(ctx context.Context, args []string) error {
	fs := newFlagSet("migrate rollback")
	root := fs.String("root", ".", "project root")
	name := fs.String("name", "", "migration name")
	fs.Usage = func() { printMigrateRollbackHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	if *name == "" {
		printMigrateRollbackHelp(os.Stderr)
		return errkind.New(errkind.KindConfiguration, "migrate rollback requires --name")
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "migrate rollback: load config", err)
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
	if err := service.Revert(ctx, db.SQLDB(), *name); err != nil {
		return errkind.Wrap(errkind.KindMigrationApplication, fmt.Sprintf("migrate rollback: %s", *name), err)
	}
	fmt.Fprintf(os.Stdout, "✓ Rolled back %s\n", *name)
	return nil
}

func cmdMigrateStatus(ctx context.Context, args []string) error {
	fs := newFlagSet("migrate status")
	root := fs.String("root", ".", "project root")
	fs.Usage = func() { printMigrateStatusHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "migrate status: load config", err)
	}
	if err := validateProjectPaths(*root, cfg, false); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "migrate status: validate project", err)
	}
	db, err := openConfiguredDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(context.Background()); err != nil {
		return errkind.Wrap(errkind.KindRuntimeQuery, "migrate status: ping database", err)
	}
	if err := ensureMigrationRegistry(context.Background(), db.SQLDB()); err != nil {
		return errkind.Wrap(errkind.KindMigrationApplication, "migrate status: ensure migration registry", err)
	}
	applied, err := appliedMigrationSet(context.Background(), db.SQLDB())
	if err != nil {
		return errkind.Wrap(errkind.KindMigrationApplication, "migrate status: read applied migrations", err)
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
		return errkind.Wrap(errkind.KindMigrationApplication, "migrate status: list migrations", err)
	}
	if len(names) == 0 {
		fmt.Fprintln(os.Stdout, "⚠ No migrations found")
		return nil
	}
	pendingCount := 0
	fmt.Fprintln(os.Stdout, "Applied:")
	for _, name := range names {
		if applied[name] {
			fmt.Fprintf(os.Stdout, "- %s\n", name)
		} else {
			pendingCount++
		}
	}
	if pendingCount == 0 {
		fmt.Fprintln(os.Stdout, "⚠ No pending migrations")
		return nil
	}
	fmt.Fprintln(os.Stdout, "Pending:")
	for _, name := range names {
		if !applied[name] {
			fmt.Fprintf(os.Stdout, "- %s\n", name)
		}
	}
	return nil
}

func cmdSchema(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printSchemaHelp(os.Stdout)
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		printSchemaHelp(os.Stdout)
		return nil
	case "check":
		return cmdSchemaCheck(ctx, args[1:])
	default:
		printSchemaHelp(os.Stderr)
		return errkind.New(errkind.KindConfiguration, fmt.Sprintf("schema: unknown command %q", args[0]))
	}
}

func cmdSeed(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printSeedHelp(os.Stdout)
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		printSeedHelp(os.Stdout)
		return nil
	case "run":
		return cmdSeedRun(ctx, args[1:])
	case "list":
		return cmdSeedList(args[1:])
	case "reset":
		return cmdSeedReset(args[1:])
	default:
		printSeedHelp(os.Stderr)
		return errkind.New(errkind.KindConfiguration, fmt.Sprintf("seed: unknown command %q", args[0]))
	}
}

func cmdSeedRun(ctx context.Context, args []string) error {
	fs := newFlagSet("seed run")
	root := fs.String("root", ".", "project root")
	fs.Usage = func() { printSeedRunHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "seed run: load config", err)
	}
	if err := validateProjectPaths(*root, cfg, false); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "seed run: validate project", err)
	}
	db, err := openConfiguredDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return errkind.Wrap(errkind.KindRuntimeQuery, "seed run: ping database", err)
	}
	if err := seed.Run(ctx, db); err != nil {
		return errkind.Wrap(errkind.KindRuntimeQuery, "seed run: execute seeders", err)
	}
	fmt.Fprintln(os.Stdout, "✓ Seed run completed")
	return nil
}

func cmdSeedList(args []string) error {
	fs := newFlagSet("seed list")
	fs.Usage = func() { printSeedListHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	names := seed.List()
	if len(names) == 0 {
		fmt.Fprintln(os.Stdout, "⚠ No seeders registered")
		return nil
	}
	for _, name := range names {
		fmt.Fprintln(os.Stdout, name)
	}
	return nil
}

func cmdSeedReset(args []string) error {
	fs := newFlagSet("seed reset")
	fs.Usage = func() { printSeedResetHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	seed.Reset()
	fmt.Fprintln(os.Stdout, "✓ Seed registry cleared")
	return nil
}

func cmdSchemaCheck(ctx context.Context, args []string) error {
	fs := newFlagSet("schema check")
	root := fs.String("root", ".", "project root")
	fs.Usage = func() { printSchemaCheckHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "schema check: load config", err)
	}
	if err := validateProjectPaths(*root, cfg, true); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "schema check: validate project", err)
	}
	db, err := openConfiguredDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return errkind.Wrap(errkind.KindRuntimeQuery, "schema check: ping database", err)
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
		return errkind.Wrap(errkind.KindRuntimeQuery, "schema check: detect drift", err)
	}
	if report != nil && !report.Clean() {
		fmt.Fprintln(os.Stdout, "⚠ Schema drift detected")
		if report.ExpectedDiff != nil {
			for _, op := range report.ExpectedDiff.Operations {
				fmt.Fprintf(os.Stdout, "- %s %s\n", op.Kind, op.Table)
			}
		}
		if report.HasSnapshotDrift() {
			fmt.Fprintln(os.Stdout, "⚠ Snapshot drift detected")
			for _, op := range report.SnapshotDiff.Operations {
				fmt.Fprintf(os.Stdout, "- %s %s\n", op.Kind, op.Table)
			}
		}
		return errkind.New(errkind.KindInvalidSchema, "schema check: drift detected; run `dorm migrate generate` and `dorm migrate run` to reconcile")
	}
	fmt.Fprintln(os.Stdout, "✓ Schema is up to date")
	return nil
}

func cmdDoctor(ctx context.Context, args []string) error {
	fs := newFlagSet("doctor")
	root := fs.String("root", ".", "project root")
	fs.Usage = func() {
		printDoctorHelp(os.Stdout)
	}
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	if err := dorm.DefaultCompatibilityPolicy().ValidateRuntime(); err != nil {
		return errkind.Wrap(errkind.KindUnsupportedFeature, "doctor: compatibility check failed; rebuild with a supported Go version", err)
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "doctor: load config", err)
	}
	if cfg.Root == "" || cfg.MigrationsDir == "" || cfg.SnapshotPath == "" {
		return errkind.New(errkind.KindConfiguration, "doctor: incomplete config; run `dorm init` to regenerate orm.json")
	}
	if cfg.Driver != postgres.New().Name() {
		return errkind.New(errkind.KindUnsupportedFeature, fmt.Sprintf("doctor: unsupported driver %q; only postgres is supported", cfg.Driver))
	}
	snapshot, err := schema.LoadSnapshot(filepath.Join(*root, cfg.SnapshotPath))
	if err != nil {
		return errkind.Wrap(errkind.KindInvalidSchema, "doctor: snapshot integrity check failed; regenerate the snapshot", err)
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
		return errkind.Wrap(errkind.KindRuntimeQuery, "doctor: connectivity check failed; verify DSN and database reachability", err)
	}
	fmt.Println("✓ Compatibility OK")
	fmt.Println("✓ Config OK")
	fmt.Println("✓ Connectivity OK")
	fmt.Println("✓ Snapshot OK")
	fmt.Println("✓ Dialect OK")
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
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, errkind.New(errkind.KindConfiguration, "database DSN is required; set dsn in orm.json")
	}
	driver := driverpostgres.New(driverpostgres.Config{
		DSN:        cfg.DSN,
		DriverName: cfg.Driver,
	})
	dorm.RegisterDriver(driver)
	return dorm.Open(ctx, driver)
}

func validateProjectPaths(root string, cfg cliConfig, requireModels bool) error {
	if strings.TrimSpace(root) == "" {
		return errkind.New(errkind.KindConfiguration, "project root is required")
	}
	if strings.TrimSpace(cfg.MigrationsDir) == "" {
		return errkind.New(errkind.KindConfiguration, "migrations_dir is missing from orm.json; run `dorm init`")
	}
	if strings.TrimSpace(cfg.SnapshotPath) == "" {
		return errkind.New(errkind.KindConfiguration, "snapshot_path is missing from orm.json; run `dorm init`")
	}
	if requireModels && strings.TrimSpace(cfg.ModelsDir) == "" {
		return errkind.New(errkind.KindConfiguration, "models_dir is missing from orm.json; run `dorm init`")
	}
	if _, err := os.Stat(filepath.Join(root, cfg.MigrationsDir)); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, fmt.Sprintf("missing migration directory %q; run `dorm init` or create it manually", cfg.MigrationsDir), err)
	}
	if requireModels {
		if _, err := os.Stat(filepath.Join(root, cfg.ModelsDir)); err != nil {
			return errkind.Wrap(errkind.KindConfiguration, fmt.Sprintf("missing model directory %q; run `dorm init` or create it manually", cfg.ModelsDir), err)
		}
	}
	return nil
}

func validateModelProjectPaths(root string, cfg cliConfig) error {
	if strings.TrimSpace(root) == "" {
		return errkind.New(errkind.KindConfiguration, "project root is required")
	}
	if strings.TrimSpace(cfg.ModelsDir) == "" {
		return errkind.New(errkind.KindConfiguration, "models_dir is missing from orm.json; run `dorm init`")
	}
	if _, err := os.Stat(filepath.Join(root, cfg.ModelsDir)); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, fmt.Sprintf("missing model directory %q; run `dorm init` or create it manually", cfg.ModelsDir), err)
	}
	return nil
}

func handleFlagError(fs *flag.FlagSet, err error) error {
	if err == nil || err == flag.ErrHelp {
		return nil
	}
	if strings.Contains(err.Error(), "flag provided but not defined") {
		return errkind.New(errkind.KindConfiguration, err.Error()+". Run with --help to see available flags.")
	}
	return errkind.Wrap(errkind.KindConfiguration, fs.Name()+": invalid flags; run with --help to see usage", err)
}

func printRootHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm - PostgreSQL-first ORM CLI")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm <command> [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init      Create project structure and config")
	fmt.Fprintln(w, "  migrate   Generate, run, rollback, or inspect migrations")
	fmt.Fprintln(w, "  schema    Check schema drift against source")
	fmt.Fprintln(w, "  seed      Run, list, or reset registered seeders")
	fmt.Fprintln(w, "  analyze   Analyze SQL and suggest query improvements")
	fmt.Fprintln(w, "  dry-run   Render a Dry Run execution report")
	fmt.Fprintln(w, "  doctor    Validate project and database readiness")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm init --root .")
	fmt.Fprintln(w, "  dorm migrate generate --root .")
	fmt.Fprintln(w, "  dorm schema check --root .")
	fmt.Fprintln(w, "  dorm seed run --root .")
	fmt.Fprintln(w, "  dorm analyze --root . --sql \"SELECT * FROM users WHERE email = $1\"")
}

func cmdAnalyze(ctx context.Context, args []string) error {
	fs := newFlagSet("analyze")
	root := fs.String("root", ".", "project root")
	sqlText := fs.String("sql", "", "SQL statement to analyze")
	sqlFile := fs.String("sql-file", "", "file containing SQL to analyze")
	tableName := fs.String("table", "", "table name to analyze")
	offsetThreshold := fs.Int("offset-threshold", analyzer.DefaultLargeOffsetThreshold, "large OFFSET threshold")
	fs.Usage = func() { printAnalyzeHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	if strings.TrimSpace(*sqlText) == "" && strings.TrimSpace(*sqlFile) == "" {
		return errkind.New(errkind.KindConfiguration, "analyze requires --sql or --sql-file")
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "analyze: load config", err)
	}
	if err := validateModelProjectPaths(*root, cfg); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "analyze: validate project", err)
	}
	sqlInput := strings.TrimSpace(*sqlText)
	if sqlInput == "" {
		data, err := os.ReadFile(*sqlFile)
		if err != nil {
			return errkind.Wrap(errkind.KindConfiguration, "analyze: read sql file", err)
		}
		sqlInput = string(data)
	}
	schemaMeta, err := schema.NewBuilder(filepath.Join(*root, cfg.ModelsDir)).Build(ctx)
	if err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "analyze: build schema", err)
	}
	report, err := analyzer.New(analyzer.Config{
		LargeOffsetThreshold: *offsetThreshold,
	}).Analyze(ctx, analyzer.Input{
		SQL:    sqlInput,
		Table:  strings.TrimSpace(*tableName),
		Schema: schemaMeta,
	})
	if err != nil {
		return errkind.Wrap(errkind.KindRuntimeQuery, "analyze: analyze sql", err)
	}
	fmt.Fprintln(os.Stdout, report.String())
	return nil
}

func cmdDryRun(args []string) error {
	fs := newFlagSet("dry-run")
	reportFile := fs.String("report", "", "execution report JSON file")
	fs.Usage = func() { printDryRunHelp(os.Stdout) }
	if err := fs.Parse(args); err != nil {
		return handleFlagError(fs, err)
	}
	var data []byte
	var err error
	if strings.TrimSpace(*reportFile) != "" {
		data, err = os.ReadFile(*reportFile)
		if err != nil {
			return errkind.Wrap(errkind.KindConfiguration, "dry-run: read report", err)
		}
	} else {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return errkind.Wrap(errkind.KindConfiguration, "dry-run: read stdin", err)
		}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		printDryRunHelp(os.Stdout)
		return nil
	}
	var report dorm.ExecutionReport
	if err := json.Unmarshal(data, &report); err != nil {
		return errkind.Wrap(errkind.KindConfiguration, "dry-run: decode report", err)
	}
	fmt.Fprint(os.Stdout, formatDryRunReport(report))
	return nil
}

func printInitHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm init")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Create the project directory structure and default orm.json.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm init --root <path>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --root string   project root (default \".\")")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm init --root .")
}

func printMigrateHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm migrate")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Generate, apply, rollback, or inspect database migrations.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm migrate <generate|run|rollback|status> [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm migrate generate --root .")
	fmt.Fprintln(w, "  dorm migrate run --root .")
	fmt.Fprintln(w, "  dorm migrate rollback --name 0001_schema --root .")
}

func printMigrateGenerateHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm migrate generate")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Generate SQL migrations from model source and snapshot state.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm migrate generate --root <path>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --root string   project root (default \".\")")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm migrate generate --root .")
}

func printMigrateRunHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm migrate run")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Apply pending migrations to the configured database.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm migrate run --root <path>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --root string   project root (default \".\")")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm migrate run --root .")
}

func printMigrateRollbackHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm migrate rollback")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Revert a named migration using its down SQL file.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm migrate rollback --name <migration> --root <path>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --name string   migration name to roll back")
	fmt.Fprintln(w, "  --root string   project root (default \".\")")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm migrate rollback --name 0001_schema --root .")
}

func printMigrateStatusHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm migrate status")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Show applied and pending migrations.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm migrate status --root <path>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --root string   project root (default \".\")")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm migrate status --root .")
}

func printSchemaHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm schema")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Inspect schema drift against the current model source.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm schema check --root <path>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm schema check --root .")
}

func printSchemaCheckHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm schema check")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Compare the live database with the generated schema snapshot.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm schema check --root <path>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --root string   project root (default \".\")")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm schema check --root .")
}

func printSeedHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm seed")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Run, list, or reset registered seeders.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm seed <run|list|reset> [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm seed run --root .")
	fmt.Fprintln(w, "  dorm seed list")
	fmt.Fprintln(w, "  dorm seed reset")
}

func printSeedRunHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm seed run")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Execute registered seeders against the configured database.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm seed run --root <path>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --root string   project root (default \".\")")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm seed run --root .")
}

func printSeedListHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm seed list")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  List registered seeders in execution order.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm seed list")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm seed list")
}

func printSeedResetHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm seed reset")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Clear the in-memory seeder registry for the current process.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm seed reset")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm seed reset")
}

func printAnalyzeHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm analyze")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Analyze generated SQL and print query-quality recommendations.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm analyze --root <path> --sql <statement>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --root string            project root (default \".\")")
	fmt.Fprintln(w, "  --sql string             SQL statement to analyze")
	fmt.Fprintln(w, "  --sql-file string        file containing SQL to analyze")
	fmt.Fprintln(w, "  --table string           table name when SQL is ambiguous")
	fmt.Fprintln(w, "  --offset-threshold int   large OFFSET threshold (default 1000)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm analyze --root . --sql \"SELECT * FROM users WHERE email = $1\"")
	fmt.Fprintln(w, "  dorm analyze --root . --table users --sql-file query.sql")
}

func printDryRunHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm dry-run")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Render a Dry Run execution report in a human-readable format.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm dry-run --report <report.json>")
	fmt.Fprintln(w, "  cat report.json | dorm dry-run")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --report string   execution report JSON file")
}

func formatDryRunReport(report dorm.ExecutionReport) string {
	var b strings.Builder
	writeDryRunSection(&b, "Access Policy")
	if len(report.AccessPolicies) == 0 {
		fmt.Fprintln(&b, "(none)")
	} else {
		for _, event := range report.AccessPolicies {
			switch event.Kind {
			case dorm.AccessPolicyEventSoftDelete:
				if event.Field != "" {
					fmt.Fprintf(&b, "✓ %s injected\n", event.Field)
				} else {
					fmt.Fprintln(&b, "✓ soft delete injected")
				}
			case dorm.AccessPolicyEventInjectedField, dorm.AccessPolicyEventInjectedPredicate:
				if event.Field != "" {
					fmt.Fprintf(&b, "✓ %s injected\n", event.Field)
				} else if event.SQL != "" {
					fmt.Fprintf(&b, "✓ %s\n", event.SQL)
				}
			case dorm.AccessPolicyEventPolicyOverride:
				fmt.Fprintf(&b, "✓ policy override: %s\n", event.Policy)
			case dorm.AccessPolicyEventInheritedPolicy:
				fmt.Fprintf(&b, "✓ policy: %s\n", event.Policy)
			}
		}
	}
	writeDryRunSection(&b, "Generated SQL")
	if strings.TrimSpace(report.SQL) == "" {
		fmt.Fprintln(&b, "(none)")
	} else {
		fmt.Fprintln(&b, report.SQL)
	}
	writeDryRunSection(&b, "Parameters")
	if len(report.Parameters) == 0 {
		fmt.Fprintln(&b, "(none)")
	} else {
		for i, param := range report.Parameters {
			fmt.Fprintf(&b, "$%d = %v\n", i+1, param)
		}
	}
	writeDryRunSection(&b, "Audit Actions")
	if len(report.AuditActions) == 0 {
		fmt.Fprintln(&b, "(none)")
	} else {
		for _, action := range report.AuditActions {
			fmt.Fprintf(&b, "✓ %s\n", action.Field)
		}
	}
	writeDryRunSection(&b, "Lifecycle Hooks")
	if len(report.LifecycleHooks) == 0 {
		fmt.Fprintln(&b, "(none)")
	} else {
		for _, hook := range report.LifecycleHooks {
			fmt.Fprintf(&b, "✓ %s\n", hook.Name)
		}
	}
	writeDryRunSection(&b, "Query Advisor")
	if len(report.QueryAdvisor) == 0 {
		fmt.Fprintln(&b, "(none)")
	} else {
		for _, finding := range report.QueryAdvisor {
			title := finding.Title
			if title == "" {
				title = finding.Code
			}
			fmt.Fprintf(&b, "⚠ %s\n", title)
			if finding.Recommendation != "" {
				fmt.Fprintf(&b, "%s\n", finding.Recommendation)
			}
		}
	}
	writeDryRunSection(&b, "Optimistic Locking")
	if report.OptimisticLocking == nil || !report.OptimisticLocking.Enabled {
		fmt.Fprintln(&b, "(none)")
	} else {
		fmt.Fprintf(&b, "✓ %s enabled\n", report.OptimisticLocking.Column)
		if report.OptimisticLocking.Current != nil {
			fmt.Fprintf(&b, "current = %v\n", report.OptimisticLocking.Current)
		}
		if report.OptimisticLocking.Next != nil {
			fmt.Fprintf(&b, "next = %v\n", report.OptimisticLocking.Next)
		}
		if report.OptimisticLocking.Conflict {
			fmt.Fprintln(&b, "⚠ conflict detected")
		}
	}
	writeDryRunSection(&b, "Execution")
	if report.ExecutionStatus == "" {
		fmt.Fprintln(&b, string(dorm.ExecutionStatusSkipped))
	} else {
		fmt.Fprintln(&b, report.ExecutionStatus)
	}
	return b.String()
}

func writeDryRunSection(b *strings.Builder, title string) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintln(b, title)
	fmt.Fprintln(b, "")
}

func printDoctorHelp(w io.Writer) {
	fmt.Fprintln(w, "dorm doctor")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Purpose:")
	fmt.Fprintln(w, "  Validate project compatibility, config, connectivity, and snapshot integrity.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dorm doctor --root <path>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --root string   project root (default \".\")")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  dorm doctor --root .")
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
