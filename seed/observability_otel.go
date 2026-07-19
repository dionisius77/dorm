package seed

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/dionisius77/dorm/dialect"
	"github.com/dionisius77/dorm/orm"
)

type seedTraceProvider interface {
	Observability() orm.ObservabilityConfig
	DriverName() string
	Dialect() dialect.Dialect
}

func traceSeedOperation(ctx context.Context, provider seedTraceProvider, spanName string, attrs []orm.Attribute, fn func(context.Context, orm.Span) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if provider == nil {
		return fn(ctx, nil)
	}
	obs := provider.Observability()
	if !obs.Tracing {
		return fn(ctx, nil)
	}
	attrs = append(seedBaseAttributes(provider), attrs...)
	start := time.Now()
	if obs.TracerProvider != nil {
		ctx, span := obs.TracerProvider.Tracer("github.com/dionisius77/dorm").Start(ctx, spanName)
		if len(attrs) > 0 {
			span.SetAttributes(attrs...)
		}
		err := fn(ctx, span)
		if span != nil {
			span.SetAttributes(orm.Attribute{Key: "seed.duration_seconds", Value: time.Since(start).Seconds()})
		}
		if err != nil {
			span.RecordError(err)
			setSeedSpanStatus(span, err)
			if obs.Logger != nil {
				obs.Logger.LogError(ctx, err)
			}
		}
		span.End()
		return err
	}
	ctx, span := otel.Tracer("github.com/dionisius77/dorm").Start(ctx, spanName)
	if len(attrs) > 0 {
		span.SetAttributes(seedToOTELAttributes(attrs)...)
	}
	err := fn(ctx, seedOtelSpanAdapter{span: span})
	if span != nil {
		span.SetAttributes(attribute.Float64("seed.duration_seconds", time.Since(start).Seconds()))
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		if obs.Logger != nil {
			obs.Logger.LogError(ctx, err)
		}
	}
	span.End()
	return err
}

func seedBaseAttributes(provider seedTraceProvider) []orm.Attribute {
	attrs := []orm.Attribute{}
	if provider == nil {
		return attrs
	}
	if driverName := provider.DriverName(); driverName != "" {
		attrs = append(attrs, orm.Attribute{Key: "driver.name", Value: driverName})
	}
	if dialect := provider.Dialect(); dialect != nil {
		attrs = append(attrs, orm.Attribute{Key: "driver.dialect", Value: dialect.Name()})
		attrs = append(attrs, orm.Attribute{Key: "db.system", Value: dialect.Name()})
	}
	return attrs
}

func setSeedSpanStatus(span orm.Span, err error) {
	if span == nil || err == nil {
		return
	}
	if statusSetter, ok := span.(interface {
		SetStatus(otelcodes.Code, string)
	}); ok {
		statusSetter.SetStatus(otelcodes.Error, err.Error())
	}
}

func seedToOTELAttributes(attrs []orm.Attribute) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, attribute.String(attr.Key, fmt.Sprint(attr.Value)))
	}
	return out
}

type seedOtelSpanAdapter struct {
	span oteltrace.Span
}

func (s seedOtelSpanAdapter) End() {
	if s.span != nil {
		s.span.End()
	}
}

func (s seedOtelSpanAdapter) RecordError(err error) {
	if s.span != nil && err != nil {
		s.span.RecordError(err)
	}
}

func (s seedOtelSpanAdapter) SetAttributes(attrs ...orm.Attribute) {
	if s.span == nil || len(attrs) == 0 {
		return
	}
	s.span.SetAttributes(seedToOTELAttributes(attrs)...)
}
