package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dionisius77/dorm/dialect"
	pgdialect "github.com/dionisius77/dorm/dialect/postgres"
	"github.com/dionisius77/dorm/driver"
	dormerrors "github.com/dionisius77/dorm/errors"
	"github.com/jackc/pgx/v5/stdlib"
)

const defaultDriverName = "postgres"

var registeredDrivers sync.Map

type Config struct {
	DSN              string
	DriverName       string
	Host             string
	Port             int
	Database         string
	Username         string
	Password         string
	SSLMode          string
	Timezone         string
	SearchPath       string
	PreflightEnabled bool
	ModelRoot        string
	SnapshotPath     string
	SchemaName       string
	Options          map[string]string
	MaxOpenConns     int
	MaxIdleConns     int
	ConnMaxLifetime  time.Duration
	ConnMaxIdleTime  time.Duration
}

type Driver struct {
	cfg     Config
	dialect dialect.Dialect
}

var _ driver.Driver = (*Driver)(nil)
var _ driver.PreflightProvider = (*Driver)(nil)

func New(cfg Config) *Driver {
	cfg = normalizeConfig(cfg)
	return &Driver{
		cfg:     cfg,
		dialect: pgdialect.New(),
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.DriverName == "" {
		cfg.DriverName = defaultDriverName
	}
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}
	if cfg.Timezone == "" {
		cfg.Timezone = "UTC"
	}
	if cfg.SearchPath == "" {
		cfg.SearchPath = "public"
	}
	if cfg.SchemaName == "" {
		cfg.SchemaName = "public"
	}
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = 25
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = 25
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = time.Hour
	}
	if cfg.ConnMaxIdleTime == 0 {
		cfg.ConnMaxIdleTime = 15 * time.Minute
	}
	return cfg
}

func (d *Driver) Validate() error {
	if d == nil {
		return dormerrors.NewDriverError(dormerrors.KindConfiguration, defaultDriverName, "validate", fmt.Errorf("nil driver"))
	}
	if d.dialect == nil {
		return dormerrors.NewDriverError(dormerrors.KindConfiguration, d.Name(), "validate", fmt.Errorf("nil dialect"))
	}
	if strings.TrimSpace(d.cfg.DSN) == "" && strings.TrimSpace(d.cfg.Database) == "" {
		return dormerrors.NewDriverError(dormerrors.KindConfiguration, d.Name(), "validate", fmt.Errorf("database is required"))
	}
	return nil
}

func (d *Driver) Name() string {
	if d == nil || strings.TrimSpace(d.cfg.DriverName) == "" {
		return defaultDriverName
	}
	return d.cfg.DriverName
}

func (d *Driver) Dialect() dialect.Dialect {
	if d == nil {
		return pgdialect.New()
	}
	if d.dialect == nil {
		d.dialect = pgdialect.New()
	}
	return d.dialect
}

func (d *Driver) PreflightConfig() driver.PreflightConfig {
	if d == nil {
		return driver.PreflightConfig{}
	}
	return driver.PreflightConfig{
		Enabled:      d.cfg.PreflightEnabled,
		Root:         d.cfg.ModelRoot,
		SnapshotPath: d.cfg.SnapshotPath,
		SchemaName:   d.cfg.SchemaName,
	}
}

func (d *Driver) Open(ctx context.Context) (*sql.DB, error) {
	_ = ctx
	if err := d.Validate(); err != nil {
		return nil, err
	}
	ensureRegistered(d.Name())
	db, err := sql.Open(d.Name(), d.dsn())
	if err != nil {
		return nil, dormerrors.NewDriverError(dormerrors.KindConfiguration, d.Name(), "open", err)
	}
	if d.cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(d.cfg.MaxOpenConns)
	}
	if d.cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(d.cfg.MaxIdleConns)
	}
	if d.cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(d.cfg.ConnMaxLifetime)
	}
	if d.cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(d.cfg.ConnMaxIdleTime)
	}
	return db, nil
}

func ensureRegistered(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultDriverName
	}
	if _, loaded := registeredDrivers.LoadOrStore(name, struct{}{}); loaded {
		return
	}
	if sqlDriverRegistered(name) {
		return
	}
	sql.Register(name, stdlib.GetDefaultDriver())
}

func sqlDriverRegistered(name string) bool {
	for _, registered := range sql.Drivers() {
		if registered == name {
			return true
		}
	}
	return false
}

func (d *Driver) dsn() string {
	if d == nil {
		return ""
	}
	if strings.TrimSpace(d.cfg.DSN) != "" {
		return d.cfg.DSN
	}
	parts := map[string]string{
		"host":        d.cfg.Host,
		"port":        strconv.Itoa(d.cfg.Port),
		"dbname":      d.cfg.Database,
		"user":        d.cfg.Username,
		"password":    d.cfg.Password,
		"sslmode":     d.cfg.SSLMode,
		"timezone":    d.cfg.Timezone,
		"search_path": d.cfg.SearchPath,
	}
	keys := []string{"host", "port", "dbname", "user", "password", "sslmode", "timezone", "search_path"}
	var b strings.Builder
	first := true
	for _, key := range keys {
		val := strings.TrimSpace(parts[key])
		if val == "" {
			continue
		}
		if !first {
			b.WriteByte(' ')
		}
		first = false
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(val))
	}
	for _, key := range sortedKeys(d.cfg.Options) {
		val := strings.TrimSpace(d.cfg.Options[key])
		if val == "" {
			continue
		}
		if !first {
			b.WriteByte(' ')
		}
		first = false
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(val))
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
