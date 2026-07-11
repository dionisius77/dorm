package schema

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
)

const otelInstrumentationName = "github.com/dionisius77/dorm"

func traceOperation(ctx context.Context, spanName string, fn func(context.Context) error) error {
	ctx, span := otel.Tracer(otelInstrumentationName).Start(ctx, spanName)
	start := time.Now()
	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	span.End()
	recordSchemaMetrics(ctx, spanName, time.Since(start), err)
	return err
}

func recordSchemaMetrics(ctx context.Context, spanName string, duration time.Duration, err error) {
	meter := otel.Meter(otelInstrumentationName)
	if meter == nil {
		return
	}
	counter, cErr := meter.Int64Counter("schema.operation.count")
	if cErr == nil {
		counter.Add(ctx, 1, otelmetric.WithAttributes())
	}
	hist, hErr := meter.Float64Histogram("schema.operation.duration")
	if hErr == nil {
		hist.Record(ctx, duration.Seconds(), otelmetric.WithAttributes())
	}
	if err != nil {
		failures, fErr := meter.Int64Counter("schema.operation.failed")
		if fErr == nil {
			failures.Add(ctx, 1, otelmetric.WithAttributes())
		}
	}
	_ = spanName
}
