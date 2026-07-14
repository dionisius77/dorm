package orm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dionisius77/dorm/errkind"
)

// TraceSQLMode controls how much SQL detail is exposed through traces.
type TraceSQLMode string

const (
	TraceSQLDisabled          TraceSQLMode = "disabled"
	TraceSQLMetadata          TraceSQLMode = "metadata"
	TraceSQLStatement         TraceSQLMode = "statement"
	TraceSQLStatementWithArgs TraceSQLMode = "statement_with_args"
)

// SQLLogMode controls how SQL statements are reported by the legacy logger path.
type SQLLogMode string

const (
	SQLLogDisabled   SQLLogMode = "disabled"
	SQLLogErrorsOnly SQLLogMode = "errors_only"
	SQLLogSlow       SQLLogMode = "slow_queries"
	SQLLogDebug      SQLLogMode = "debug"
	SQLLogTrace      SQLLogMode = "trace"
)

// ObservabilityConfig configures tracing, metrics, and SQL visibility.
type ObservabilityConfig struct {
	Tracing            bool
	Metrics            bool
	TraceSQL           TraceSQLMode
	SQLLogging         SQLLogMode
	SlowQueryThreshold time.Duration
	MaskParameters     bool
	MaskedFields       []string
	Logger             ObservabilityLogger
	TracerProvider     TracerProvider
	MeterProvider      MeterProvider
}

// Observability is an alias kept for the public API documented in ADR-026.
type Observability = ObservabilityConfig

// SQLLogEntry captures a single SQL log event.
type SQLLogEntry struct {
	SQL          string
	Args         []any
	Duration     time.Duration
	AffectedRows int64
	Timestamp    time.Time
	Err          error
	Slow         bool
	Visibility   TraceSQLMode
	Driver       string
	Dialect      string
	Operation    string
	Table        string
}

// ObservabilityLogger receives SQL logs and error events.
type ObservabilityLogger interface {
	LogSQL(context.Context, SQLLogEntry)
	LogError(context.Context, error)
}

// TracerProvider creates named tracers.
type TracerProvider interface {
	Tracer(name string) Tracer
}

// Tracer starts spans.
type Tracer interface {
	Start(context.Context, string, ...SpanOption) (context.Context, Span)
}

// Span represents an active trace span.
type Span interface {
	End()
	RecordError(error)
	SetAttributes(...Attribute)
}

// SpanOption configures span creation.
type SpanOption interface {
	spanOption()
}

type spanOption struct{}

func (spanOption) spanOption() {}

// Attribute is a key-value pair attached to spans and metrics.
type Attribute struct {
	Key   string
	Value any
}

// MeterProvider creates named meters.
type MeterProvider interface {
	Meter(name string) Meter
}

// Meter creates counters and histograms.
type Meter interface {
	Counter(name string) Counter
	Histogram(name string) Histogram
}

// Counter records monotonic values.
type Counter interface {
	Add(context.Context, float64, ...Attribute)
}

// Histogram records duration or size distributions.
type Histogram interface {
	Record(context.Context, float64, ...Attribute)
}

// DefaultObservabilityConfig returns the default observability configuration.
func DefaultObservabilityConfig() ObservabilityConfig {
	return ObservabilityConfig{
		TraceSQL:   TraceSQLMetadata,
		SQLLogging: SQLLogDisabled,
	}
}

// Validate checks that the configuration is internally consistent.
func (c ObservabilityConfig) Validate() error {
	switch c.TraceSQL {
	case TraceSQLDisabled, TraceSQLMetadata, TraceSQLStatement, TraceSQLStatementWithArgs, "":
	default:
		return errkind.New(errkind.KindConfiguration, fmt.Sprintf("orm: invalid sql trace mode %q", c.TraceSQL))
	}
	switch c.SQLLogging {
	case SQLLogDisabled, SQLLogErrorsOnly, SQLLogSlow, SQLLogDebug, SQLLogTrace, "":
	default:
		return errkind.New(errkind.KindConfiguration, fmt.Sprintf("orm: invalid sql logging mode %q", c.SQLLogging))
	}
	if c.SlowQueryThreshold < 0 {
		return errkind.New(errkind.KindConfiguration, "orm: slow query threshold must be non-negative")
	}
	return nil
}

// Enabled reports whether any observability feature is active.
func (c ObservabilityConfig) Enabled() bool {
	return c.Tracing || c.Metrics || (c.Tracing && c.TraceSQL != TraceSQLDisabled) || c.SQLLogging != SQLLogDisabled || c.Logger != nil
}

// Normalized returns a copy with defaults applied.
func (c ObservabilityConfig) Normalized() ObservabilityConfig {
	out := c
	if out.TraceSQL == "" {
		out.TraceSQL = TraceSQLMetadata
	}
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

// ShouldMask reports whether a field name should be redacted in logs.
func (c ObservabilityConfig) ShouldMask(field string) bool {
	field = strings.ToLower(strings.TrimSpace(field))
	for _, candidate := range c.Normalized().MaskedFields {
		if strings.ToLower(candidate) == field {
			return true
		}
	}
	return false
}
