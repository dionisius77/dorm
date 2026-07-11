package migrate

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
	recordMetrics(ctx, time.Since(start), err)
	return err
}

func recordMetrics(ctx context.Context, duration time.Duration, err error) {
	meter := otel.Meter(otelInstrumentationName)
	if meter == nil {
		return
	}
	if counter, cErr := meter.Int64Counter("migrate.operation.count"); cErr == nil {
		counter.Add(ctx, 1, otelmetric.WithAttributes())
	}
	if hist, hErr := meter.Float64Histogram("migrate.operation.duration"); hErr == nil {
		hist.Record(ctx, duration.Seconds(), otelmetric.WithAttributes())
	}
	if err != nil {
		if failures, fErr := meter.Int64Counter("migrate.operation.failed"); fErr == nil {
			failures.Add(ctx, 1, otelmetric.WithAttributes())
		}
	}
}
