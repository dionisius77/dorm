package dorm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	otel "go.opentelemetry.io/otel"
	otelattribute "go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

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
	var db *DB
	err := traceConnectionOperation(ctx, cfg.observability, drv, "db.connect", func(ctx context.Context) error {
		if err := drv.Validate(); err != nil {
			return errkind.Wrap(errkind.KindConfiguration, "dorm: validate driver", err)
		}
		sqlDB, err := drv.Open(ctx)
		if err != nil {
			return errkind.Wrap(errkind.KindConfiguration, "dorm: open driver", err)
		}
		if sqlDB == nil {
			return errkind.New(errkind.KindConfiguration, "dorm: driver returned nil db")
		}
		db = orm.New(orm.Config{
			DB:            sqlDB,
			Context:       ctx,
			Dialect:       drv.Dialect(),
			DriverName:    drv.Name(),
			Observability: cfg.observability,
		})
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return errkind.Wrap(errkind.KindRuntimeQuery, "dorm: ping database", err)
		}
		if err := runOpenPreflight(ctx, drv, db); err != nil {
			_ = db.Close()
			return err
		}
		return nil
	})
	if err != nil {
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
	inspector, err := inspectorFromDriver(drv)
	if err != nil {
		return err
	}
	report, err := schema.DetectDriftFromSource(ctx, cfg.Root, inspector, db.SQLDB(), cfg.SchemaName, cfg.SnapshotPath)
	if err != nil {
		return errkind.Wrap(errkind.KindRuntimeQuery, "dorm: schema preflight", err)
	}
	if report != nil && !report.Clean() {
		return errkind.New(errkind.KindInvalidSchema, "dorm: schema preflight detected drift")
	}
	return nil
}

type inspectorProvider interface {
	Inspector() schema.Inspector
}

func inspectorFromDriver(drv driver.Driver) (schema.Inspector, error) {
	provider, ok := drv.(inspectorProvider)
	if !ok {
		return nil, errkind.New(errkind.KindConfiguration, "dorm: driver does not provide a schema inspector")
	}
	inspector := provider.Inspector()
	if inspector == nil {
		return nil, errkind.New(errkind.KindConfiguration, "dorm: driver returned a nil schema inspector")
	}
	return inspector, nil
}

func traceConnectionOperation(ctx context.Context, obs orm.ObservabilityConfig, drv driver.Driver, spanName string, fn func(context.Context) error) error {
	if !obs.Tracing {
		return fn(ctx)
	}
	attrs := connectionSpanAttributes(drv, spanName)
	if obs.TracerProvider != nil {
		ctx, span := obs.TracerProvider.Tracer("github.com/dionisius77/dorm").Start(ctx, spanName)
		span.SetAttributes(attrs...)
		err := fn(ctx)
		if err != nil {
			span.RecordError(err)
			setConnectionSpanStatus(span, err)
		}
		span.End()
		return err
	}
	ctx, span := otel.Tracer("github.com/dionisius77/dorm").Start(ctx, spanName)
	span.SetAttributes(convertConnectionAttrs(attrs)...)
	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	span.End()
	return err
}

func connectionSpanAttributes(drv driver.Driver, spanName string) []orm.Attribute {
	attrs := []orm.Attribute{
		{Key: "orm.operation", Value: connectionOperationName(spanName)},
		{Key: "db.operation", Value: connectionOperationName(spanName)},
	}
	if drv == nil {
		return attrs
	}
	attrs = append(attrs,
		orm.Attribute{Key: "driver.name", Value: drv.Name()},
		orm.Attribute{Key: "driver.dialect", Value: drv.Dialect().Name()},
	)
	if provider, ok := drv.(driver.ConnectionInfoProvider); ok {
		info := provider.ConnectionInfo()
		if info.System != "" {
			attrs = append(attrs, orm.Attribute{Key: "db.system", Value: info.System})
		}
		if info.Name != "" {
			attrs = append(attrs, orm.Attribute{Key: "db.name", Value: info.Name})
		}
		if info.Namespace != "" {
			attrs = append(attrs, orm.Attribute{Key: "db.namespace", Value: info.Namespace})
		}
		if info.ServerAddress != "" {
			attrs = append(attrs, orm.Attribute{Key: "server.address", Value: info.ServerAddress})
		}
		if info.ServerPort > 0 {
			attrs = append(attrs, orm.Attribute{Key: "server.port", Value: info.ServerPort})
		}
	}
	return attrs
}

func setConnectionSpanStatus(span orm.Span, err error) {
	if span == nil || err == nil {
		return
	}
	if statusSetter, ok := span.(interface {
		SetStatus(otelcodes.Code, string)
	}); ok {
		statusSetter.SetStatus(otelcodes.Error, err.Error())
	}
}

func connectionOperationName(spanName string) string {
	switch spanName {
	case "db.connect":
		return "connect"
	case "db.ping":
		return "ping"
	case "db.close":
		return "close"
	default:
		return strings.TrimPrefix(spanName, "db.")
	}
}

func convertConnectionAttrs(attrs []orm.Attribute) []otelattribute.KeyValue {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]otelattribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, otelattribute.String(attr.Key, fmt.Sprint(attr.Value)))
	}
	return out
}
