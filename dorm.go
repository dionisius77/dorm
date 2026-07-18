package dorm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dionisius77/dorm/driver"
	"github.com/dionisius77/dorm/errkind"
	dormerrors "github.com/dionisius77/dorm/errors"
	"github.com/dionisius77/dorm/orm"
	"github.com/dionisius77/dorm/schema"
)

type DB = orm.DB
type DryRunSession = orm.DryRunSession
type ExecutionReport = orm.ExecutionReport
type ExecutionStatement = orm.ExecutionStatement
type ExecutionStatus = orm.ExecutionStatus
type AccessPolicyEvent = orm.AccessPolicyEvent
type AccessPolicyEventKind = orm.AccessPolicyEventKind
type AuditAction = orm.AuditAction
type LifecycleHookEvent = orm.LifecycleHookEvent
type QueryAdvisorFinding = orm.QueryAdvisorFinding
type QueryAdvisorInput = orm.QueryAdvisorInput
type QueryAdvisorReport = orm.QueryAdvisorReport
type QueryAdvisor = orm.QueryAdvisor
type OptimisticLockingInfo = orm.OptimisticLockingInfo

const (
	ExecutionStatusSkipped             = orm.ExecutionStatusSkipped
	AccessPolicyEventInjectedPredicate = orm.AccessPolicyEventInjectedPredicate
	AccessPolicyEventInjectedField     = orm.AccessPolicyEventInjectedField
	AccessPolicyEventInheritedPolicy   = orm.AccessPolicyEventInheritedPolicy
	AccessPolicyEventPolicyOverride    = orm.AccessPolicyEventPolicyOverride
	AccessPolicyEventSoftDelete        = orm.AccessPolicyEventSoftDelete
)

var (
	ErrNotFound             = dormerrors.ErrNotFound
	ErrAlreadyExists        = dormerrors.ErrAlreadyExists
	ErrConflict             = dormerrors.ErrConflict
	ErrInvalidModel         = dormerrors.ErrInvalidModel
	ErrInvalidRelationship  = dormerrors.ErrInvalidRelationship
	ErrMigrationRequired    = dormerrors.ErrMigrationRequired
	ErrSchemaDrift          = dormerrors.ErrSchemaDrift
	ErrInvalidContext       = dormerrors.ErrInvalidContext
	ErrMissingCompany       = dormerrors.ErrMissingCompany
	ErrPolicyDenied         = dormerrors.ErrPolicyDenied
	ErrSeedConflict         = dormerrors.ErrSeedConflict
	ErrDriverNotRegistered  = dormerrors.ErrDriverNotRegistered
	ErrUnsupportedDialect   = dormerrors.ErrUnsupportedDialect
	ErrTransactionClosed    = dormerrors.ErrTransactionClosed
	ErrCommitFailed         = dormerrors.ErrCommitFailed
	ErrRollbackFailed       = dormerrors.ErrRollbackFailed
	ErrOptimisticLockFailed = dormerrors.ErrOptimisticLockFailed
	ErrRawSQLPolicyRequired = dormerrors.ErrRawSQLPolicyRequired
)

type OpenOption func(*openConfig)

type openConfig struct {
	observability orm.ObservabilityConfig
}

func WithObservability(cfg orm.ObservabilityConfig) OpenOption {
	return func(o *openConfig) {
		o.observability = cfg
	}
}

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

func Open(ctx context.Context, drv driver.Driver, opts ...OpenOption) (*DB, error) {
	if drv == nil {
		drv = RegisteredDriver()
	}
	if drv == nil {
		return nil, fmt.Errorf("dorm: no driver registered: %w", dormerrors.ErrDriverNotRegistered)
	}
	cfg := openConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
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
		DB:            sqlDB,
		Dialect:       drv.Dialect(),
		DriverName:    drv.Name(),
		Observability: cfg.observability,
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
