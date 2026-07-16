package orm

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"github.com/dionisius77/dorm/dialect"
	dormerrors "github.com/dionisius77/dorm/errors"
)

// RawBuilder executes explicit SQL with an opt-in policy bypass.
type RawBuilder struct {
	db            *DB
	ctx           context.Context
	query         string
	args          []any
	withoutPolicy bool
}

// Raw creates a Raw SQL builder from a DB handle.
func (db *DB) Raw(ctx context.Context, query string, args ...any) *RawBuilder {
	return newRawBuilder(db, ctx, query, args...)
}

// Raw creates a Raw SQL builder from a session handle.
func (s *Session) Raw(ctx context.Context, query string, args ...any) *RawBuilder {
	if s == nil {
		return newRawBuilder(nil, ctx, query, args...)
	}
	return newRawBuilder(s.db, ctx, query, args...)
}

func newRawBuilder(db *DB, ctx context.Context, query string, args ...any) *RawBuilder {
	if ctx == nil {
		if db != nil {
			ctx = db.currentContext()
		} else {
			ctx = context.Background()
		}
	}
	return &RawBuilder{
		db:    db,
		ctx:   ctx,
		query: query,
		args:  append([]any(nil), args...),
	}
}

// WithoutPolicy explicitly acknowledges that Raw SQL bypasses access policy.
func (r *RawBuilder) WithoutPolicy() *RawBuilder {
	if r == nil {
		return nil
	}
	cp := *r
	cp.withoutPolicy = true
	return &cp
}

// Scan executes a Raw SQL query and scans the result into a destination slice.
func (r *RawBuilder) Scan(dest any) error {
	return r.run("scan", "db.raw.scan", func(ctx context.Context, sqlText string) (int64, error) {
		before := sliceLength(dest)
		rows, err := r.db.queryContext(ctx, sqlText, r.args...)
		if err != nil {
			return -1, err
		}
		defer rows.Close()
		if err := scanIntoSlice(rows, dest, nil); err != nil {
			return -1, err
		}
		after := sliceLength(dest)
		if before >= 0 && after >= before {
			return int64(after - before), nil
		}
		return -1, nil
	})
}

// Exec executes a Raw SQL statement and returns the driver result.
func (r *RawBuilder) Exec() (sql.Result, error) {
	var result sql.Result
	err := r.run("exec", "db.raw.exec", func(ctx context.Context, sqlText string) (int64, error) {
		res, err := r.db.execContext(ctx, sqlText, r.args...)
		if err != nil {
			return -1, err
		}
		result = res
		if res == nil {
			return -1, nil
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return -1, nil
		}
		return rowsAffected, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *RawBuilder) run(operation, spanName string, fn func(context.Context, string) (int64, error)) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("orm: nil db")
	}
	if r.db.dialect == nil {
		return fmt.Errorf("orm: nil dialect")
	}
	sqlText := dialect.BindPlaceholders(r.query, r.db.dialect)
	entry := SQLLogEntry{
		SQL:          sqlText,
		Args:         append([]any(nil), r.args...),
		Timestamp:    time.Now().UTC(),
		Visibility:   r.db.observability.TraceSQL,
		Driver:       r.db.driverName,
		Dialect:      r.db.dialectName(),
		Operation:    "raw_" + operation,
		AffectedRows: -1,
	}
	attrs := append([]Attribute{{Key: "orm.raw", Value: true}, {Key: "orm.operation", Value: entry.Operation}}, sqlTraceVisibilityAttrs(entry, r.db)...)
	return r.db.traceWithSpan(r.ctx, spanName, attrs, func(ctx context.Context, span Span) error {
		if !r.withoutPolicy {
			return dormerrors.NewRawSQLPolicyError(operation, rawSQLPolicyHint(operation), nil)
		}
		rowsAffected, err := fn(ctx, sqlText)
		entry.Duration = time.Since(entry.Timestamp)
		entry.AffectedRows = rowsAffected
		entry.Slow = isSlowQuery(entry.Duration, r.db.observability.SlowQueryThreshold)
		if err != nil {
			entry.Err = err
			if span != nil {
				span.RecordError(err)
			}
			if span != nil {
				span.SetAttributes(sqlTraceVisibilityAttrs(entry, r.db)...)
			}
			return err
		}
		if span != nil {
			span.SetAttributes(sqlTraceVisibilityAttrs(entry, r.db)...)
		}
		return nil
	})
}

func rawSQLPolicyHint(operation string) string {
	switch operation {
	case "exec":
		return "Did you mean: db.Raw(ctx, query, args...).WithoutPolicy().Exec()"
	default:
		return "Did you mean: db.Raw(ctx, query, args...).WithoutPolicy().Scan(...)"
	}
}

func sliceLength(v any) int {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return -1
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return -1
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice {
		return -1
	}
	return rv.Len()
}
