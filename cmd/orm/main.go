package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"dorm/dialect/postgres"
	"dorm/migrate"
	"dorm/schema"
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
		return cmdMigrateStatus(args[1:])
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
	db, err := sql.Open(cfg.Driver, cfg.DSN)
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
	return service.Run(ctx, db)
}

func cmdMigrateRevert(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate revert", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	name := fs.String("name", "", "migration name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("migrate revert requires --name")
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return err
	}
	db, err := sql.Open(cfg.Driver, cfg.DSN)
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
	return service.Revert(ctx, db, *name)
}

func cmdMigrateStatus(args []string) error {
	fs := flag.NewFlagSet("migrate status", flag.ContinueOnError)
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
	names, err := service.Status()
	if err != nil {
		return err
	}
	for _, name := range names {
		fmt.Println(name)
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
	service := migrate.NewService(migrate.Config{
		Root:          filepath.Join(*root, cfg.ModelsDir),
		MigrationsDir: filepath.Join(*root, cfg.MigrationsDir),
		SnapshotPath:  filepath.Join(*root, cfg.SnapshotPath),
		SchemaName:    cfg.SchemaName,
		Dialect:       postgres.New(),
		Inspector:     schema.PostgresInspector{},
	})
	generated, err := service.Generate(ctx)
	if err != nil {
		return err
	}
	if generated.Diff != nil && !generated.Diff.Empty() {
		fmt.Fprintln(os.Stdout, "Schema Drift Detected")
		for _, op := range generated.Diff.Operations {
			fmt.Fprintf(os.Stdout, "- %s %s\n", op.Kind, op.Table)
		}
		return fmt.Errorf("schema drift detected")
	}
	fmt.Fprintln(os.Stdout, "Schema OK")
	return nil
}

func cmdDoctor(ctx context.Context, args []string) error {
	_ = ctx
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(filepath.Join(*root, defaultConfig().ConfigFile))
	if err != nil {
		return err
	}
	if cfg.Root == "" || cfg.MigrationsDir == "" || cfg.SnapshotPath == "" {
		return fmt.Errorf("doctor: incomplete config")
	}
	fmt.Println("doctor: config ok")
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
