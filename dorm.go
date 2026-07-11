package dorm

import (
	"context"
	"sync"

	"github.com/dionisius77/dorm/driver"
	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/orm"
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
	return db, nil
}
