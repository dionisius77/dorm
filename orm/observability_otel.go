package orm

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const otelInstrumentationName = "github.com/dionisius77/dorm"

func (db *DB) traceOperation(ctx context.Context, spanName string, attrs []Attribute, fn func(context.Context) error) error {
	if db == nil || !db.observability.Tracing {
		return fn(ctx)
	}
	if db.observability.TracerProvider != nil {
		ctx, span := db.observability.TracerProvider.Tracer(otelInstrumentationName).Start(ctx, spanName)
		ensureSpanAttrs(span, attrs)
		err := fn(ctx)
		setSpanError(span, err)
		if err != nil && db.observability.Logger != nil {
			db.observability.Logger.LogError(ctx, err)
		}
		span.End()
		return err
	}
	ctx, span := otel.Tracer(otelInstrumentationName).Start(ctx, spanName)
	if len(attrs) > 0 {
		span.SetAttributes(toOTELAttributes(attrs)...)
	}
	err := fn(ctx)
	setOTELSpanError(span, err)
	if err != nil && db.observability.Logger != nil {
		db.observability.Logger.LogError(ctx, err)
	}
	span.End()
	return err
}

func (db *DB) metricProvider() Meter {
	if db == nil || !db.observability.Metrics {
		return nil
	}
	if db.observability.MeterProvider != nil {
		return db.observability.MeterProvider.Meter(otelInstrumentationName)
	}
	return otelMeter{meter: otel.Meter(otelInstrumentationName)}
}

func (db *DB) recordExecMetrics(ctx context.Context, duration time.Duration, err error, rows int64) {
	meter := db.metricProvider()
	if meter == nil {
		return
	}
	meter.Counter("db.operation.count").Add(ctx, 1, Attribute{Key: "orm.operation", Value: "exec"})
	meter.Histogram("db.operation.duration").Record(ctx, duration.Seconds(), Attribute{Key: "orm.operation", Value: "exec"})
	if err != nil {
		meter.Counter("db.operation.failed").Add(ctx, 1, Attribute{Key: "orm.operation", Value: "exec"})
	}
	if rows >= 0 {
		meter.Counter("db.rows.affected").Add(ctx, float64(rows), Attribute{Key: "orm.operation", Value: "exec"})
	}
}

func (db *DB) recordQueryMetrics(ctx context.Context, duration time.Duration, err error, rows int64) {
	meter := db.metricProvider()
	if meter == nil {
		return
	}
	meter.Counter("db.operation.count").Add(ctx, 1, Attribute{Key: "orm.operation", Value: "query"})
	meter.Histogram("db.operation.duration").Record(ctx, duration.Seconds(), Attribute{Key: "orm.operation", Value: "query"})
	if err != nil {
		meter.Counter("db.operation.failed").Add(ctx, 1, Attribute{Key: "orm.operation", Value: "query"})
	}
	if rows >= 0 {
		meter.Counter("db.rows.returned").Add(ctx, float64(rows), Attribute{Key: "orm.operation", Value: "query"})
	}
}

func (db *DB) logSQL(ctx context.Context, entry SQLLogEntry) {
	if db == nil || db.observability.Logger == nil || db.observability.SQLLogging == SQLLogDisabled {
		return
	}
	switch db.observability.SQLLogging {
	case SQLLogErrorsOnly:
		if entry.Err == nil {
			return
		}
	case SQLLogSlow:
		if !entry.Slow {
			return
		}
	case SQLLogDebug, SQLLogTrace:
	}
	if db.observability.MaskParameters {
		entry.Args = maskSQLArgs(db.observability, entry.Args)
	}
	db.observability.Logger.LogSQL(ctx, entry)
}

func maskSQLArgs(cfg ObservabilityConfig, args []any) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, len(args))
	for i, arg := range args {
		out[i] = maskSQLValue(cfg, arg)
	}
	return out
}

func maskSQLValue(cfg ObservabilityConfig, value any) any {
	switch v := value.(type) {
	case string:
		if cfg.ShouldMask(v) {
			return "***"
		}
		return v
	case []byte:
		return "***"
	default:
		return value
	}
}

func toOTELAttributes(attrs []Attribute) []attribute.KeyValue {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, attribute.String(attr.Key, fmt.Sprint(attr.Value)))
	}
	return out
}

func setSpanError(span Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
}

func ensureSpanAttrs(span Span, attrs []Attribute) {
	if span == nil || len(attrs) == 0 {
		return
	}
	span.SetAttributes(attrs...)
}

func setOTELSpanError(span oteltrace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(otelcodes.Error, err.Error())
}

func isSlowQuery(duration time.Duration, threshold time.Duration) bool {
	return threshold > 0 && duration >= threshold
}

type otelMeter struct {
	meter otelmetric.Meter
}

func (m otelMeter) Counter(name string) Counter {
	if m.meter == nil {
		return noopCounter{}
	}
	counter, err := m.meter.Int64Counter(name)
	if err != nil {
		return noopCounter{}
	}
	return otelCounter{counter: counter}
}

func (m otelMeter) Histogram(name string) Histogram {
	if m.meter == nil {
		return noopHistogram{}
	}
	hist, err := m.meter.Float64Histogram(name)
	if err != nil {
		return noopHistogram{}
	}
	return otelHistogram{hist: hist}
}

type otelCounter struct {
	counter otelmetric.Int64Counter
}

func (c otelCounter) Add(ctx context.Context, value float64, attrs ...Attribute) {
	if c.counter == nil {
		return
	}
	c.counter.Add(ctx, int64(value), otelmetric.WithAttributes(toOTELAttributes(attrs)...))
}

type otelHistogram struct {
	hist otelmetric.Float64Histogram
}

func (h otelHistogram) Record(ctx context.Context, value float64, attrs ...Attribute) {
	if h.hist == nil {
		return
	}
	h.hist.Record(ctx, value, otelmetric.WithAttributes(toOTELAttributes(attrs)...))
}

type noopCounter struct{}

func (noopCounter) Add(context.Context, float64, ...Attribute) {}

type noopHistogram struct{}

func (noopHistogram) Record(context.Context, float64, ...Attribute) {}
