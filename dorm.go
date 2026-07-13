package dorm

import (
	"context"
	"strings"
	"sync"

	"github.com/dionisius77/dorm/driver"
	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/orm"
	"github.com/dionisius77/dorm/schema"
)

type DB = orm.DB

var (
	registeredDriverMu sync.RWMutex
	registeredDriver   driver.Driver
)

func RegisterDriver(d driver.Driver) {
	if d == nil {
		return
	}
	registeredDriverMu.Lock()
	registeredDriver = d
	registeredDriverMu.Unlock()
}

func RegisteredDriver() driver.Driver {
	registeredDriverMu.RLock()
	defer registeredDriverMu.RUnlock()
	return registeredDriver
}

func Open(ctx context.Context, drivers ...driver.Driver) (*DB, error) {
	var drv driver.Driver
	switch len(drivers) {
	case 0:
		drv = RegisteredDriver()
	default:
		drv = drivers[0]
		RegisterDriver(drv)
	}
	if drv == nil {
		return nil, errkind.New(errkind.KindConfiguration, "dorm: no driver registered")
	}
	if err := drv.Validate(); err != nil {
		return nil, errkind.Wrap(errkind.KindConfiguration, "dorm: validate driver", err)
	}
	sqlDB, err := drv.Open(ctx)
	if err != nil {
		return nil, errkind.Wrap(errkind.KindConfiguration, "dorm: open driver", err)
	}
	if sqlDB == nil {
		return nil, errkind.New(errkind.KindConfiguration, "dorm: driver returned nil db")
	}
	db := orm.New(orm.Config{
		DB:      sqlDB,
		Dialect: drv.Dialect(),
	})
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, errkind.Wrap(errkind.KindRuntimeQuery, "dorm: ping database", err)
	}
	if err := runOpenPreflight(ctx, drv, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func runOpenPreflight(ctx context.Context, drv driver.Driver, db *DB) error {
	provider, ok := drv.(driver.PreflightProvider)
	if !ok {
		return nil
	}
	cfg := provider.PreflightConfig()
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.Root) == "" {
		return errkind.New(errkind.KindConfiguration, "dorm: schema preflight requires model root")
	}
	report, err := schema.DetectDriftFromSource(ctx, cfg.Root, schema.PostgresInspector{}, db.SQLDB(), cfg.SchemaName, cfg.SnapshotPath)
	if err != nil {
		return errkind.Wrap(errkind.KindRuntimeQuery, "dorm: schema preflight", err)
	}
	if report != nil && !report.Clean() {
		return errkind.New(errkind.KindInvalidSchema, "dorm: schema preflight detected drift")
	}
	return nil
}
