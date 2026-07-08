package orm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type SQLLogMode string

const (
	SQLLogDisabled   SQLLogMode = "disabled"
	SQLLogErrorsOnly SQLLogMode = "errors_only"
	SQLLogSlow       SQLLogMode = "slow_queries"
	SQLLogDebug      SQLLogMode = "debug"
	SQLLogTrace      SQLLogMode = "trace"
)

type ObservabilityConfig struct {
	Tracing            bool
	Metrics            bool
	SQLLogging         SQLLogMode
	SlowQueryThreshold time.Duration
	MaskParameters     bool
	MaskedFields       []string
	Logger             ObservabilityLogger
	TracerProvider     TracerProvider
	MeterProvider      MeterProvider
}

type SQLLogEntry struct {
	SQL          string
	Args         []any
	Duration     time.Duration
	AffectedRows int64
	Timestamp    time.Time
	Err          error
	Slow         bool
}

type ObservabilityLogger interface {
	LogSQL(context.Context, SQLLogEntry)
	LogError(context.Context, error)
}

type TracerProvider interface {
	Tracer(name string) Tracer
}

type Tracer interface {
	Start(context.Context, string, ...SpanOption) (context.Context, Span)
}

type Span interface {
	End()
	RecordError(error)
	SetAttributes(...Attribute)
}

type SpanOption interface {
	spanOption()
}

type spanOption struct{}

func (spanOption) spanOption() {}

type Attribute struct {
	Key   string
	Value any
}

type MeterProvider interface {
	Meter(name string) Meter
}

type Meter interface {
	Counter(name string) Counter
	Histogram(name string) Histogram
}

type Counter interface {
	Add(context.Context, float64, ...Attribute)
}

type Histogram interface {
	Record(context.Context, float64, ...Attribute)
}

func DefaultObservabilityConfig() ObservabilityConfig {
	return ObservabilityConfig{
		SQLLogging: SQLLogDisabled,
	}
}

func (c ObservabilityConfig) Validate() error {
	switch c.SQLLogging {
	case SQLLogDisabled, SQLLogErrorsOnly, SQLLogSlow, SQLLogDebug, SQLLogTrace, "":
	default:
		return fmt.Errorf("orm: invalid sql logging mode %q", c.SQLLogging)
	}
	if c.SlowQueryThreshold < 0 {
		return fmt.Errorf("orm: slow query threshold must be non-negative")
	}
	return nil
}

func (c ObservabilityConfig) Enabled() bool {
	return c.Tracing || c.Metrics || c.SQLLogging != SQLLogDisabled || c.Logger != nil
}

func (c ObservabilityConfig) Normalized() ObservabilityConfig {
	out := c
	if out.SQLLogging == "" {
		out.SQLLogging = SQLLogDisabled
	}
	fields := append([]string{}, defaultMaskedFields()...)
	fields = append(fields, out.MaskedFields...)
	out.MaskedFields = fields
	return out
}

func defaultMaskedFields() []string {
	return []string{
		"password",
		"access_token",
		"refresh_token",
		"authorization",
		"cookie",
		"secret",
		"api_key",
		"jwt",
	}
}

func (c ObservabilityConfig) ShouldMask(field string) bool {
	field = strings.ToLower(strings.TrimSpace(field))
	for _, candidate := range c.Normalized().MaskedFields {
		if strings.ToLower(candidate) == field {
			return true
		}
	}
	return false
}
