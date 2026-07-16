package orm

import (
	"context"
	"fmt"
	"reflect"

	"github.com/dionisius77/dorm/errkind"
	dormerrors "github.com/dionisius77/dorm/errors"
)

func (s *Session) CreateMany(models any) error {
	return s.batchWrite("create_many", models, func(tx *Session, ctx context.Context, model any) error {
		return tx.create(ctx, model)
	})
}

func (s *Session) UpdateMany(models any) error {
	return s.batchWrite("update_many", models, func(tx *Session, ctx context.Context, model any) error {
		return tx.update(ctx, model, nil, true)
	})
}

func (s *Session) DeleteMany(models any) error {
	return s.batchWrite("delete_many", models, func(tx *Session, ctx context.Context, model any) error {
		return tx.delete(ctx, model)
	})
}

func (s *Session) batchWrite(operation string, models any, fn func(*Session, context.Context, any) error) error {
	if s == nil || s.db == nil {
		return errkind.New(errkind.KindConfiguration, "orm: nil session")
	}
	items, err := normalizeBatchModels(models)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	batchSize := s.db.batchSizeLimit(len(items))
	if batchSize <= 0 {
		batchSize = len(items)
	}
	return s.db.traceWithSpan(s.ctx, "db.batch", []Attribute{
		{Key: "orm.operation", Value: operation},
		{Key: "orm.batch.size", Value: len(items)},
		{Key: "orm.batch.chunk_size", Value: batchSize},
	}, func(ctx context.Context, span Span) error {
		addBatchEvent(span, "batch.start", Attribute{Key: "orm.operation", Value: operation}, Attribute{Key: "orm.batch.size", Value: len(items)})
		return s.db.Transaction(ctx, func(tx *DB) error {
			txSession := tx.session()
			for start := 0; start < len(items); start += batchSize {
				end := start + batchSize
				if end > len(items) {
					end = len(items)
				}
				addBatchEvent(span, "batch.chunk", Attribute{Key: "orm.batch.start", Value: start}, Attribute{Key: "orm.batch.end", Value: end})
				for i := start; i < end; i++ {
					if err := fn(txSession, ctx, items[i]); err != nil {
						addBatchEvent(span, "batch.error", Attribute{Key: "orm.batch.index", Value: i})
						return fmt.Errorf("%s[%d]: %w", operation, i, err)
					}
				}
			}
			addBatchEvent(span, "batch.complete", Attribute{Key: "orm.operation", Value: operation}, Attribute{Key: "orm.batch.size", Value: len(items)})
			return nil
		})
	})
}

func addBatchEvent(span Span, name string, attrs ...Attribute) {
	if span == nil {
		return
	}
	if eventer, ok := span.(interface {
		AddEvent(string, ...Attribute)
	}); ok {
		eventer.AddEvent(name, attrs...)
	}
}

func normalizeBatchModels(models any) ([]any, error) {
	if models == nil {
		return nil, errkind.New(errkind.KindConfiguration, "orm: batch models must be a slice")
	}
	rv := reflect.ValueOf(models)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, errkind.New(errkind.KindConfiguration, "orm: batch models must be a slice")
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, errkind.New(errkind.KindInvalidSchema, "orm: batch models must be a slice or array")
	}
	out := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i)
		if item.Kind() == reflect.Pointer {
			if item.IsNil() {
				return nil, dormerrors.ErrInvalidModel
			}
			if item.Elem().Kind() != reflect.Struct {
				return nil, dormerrors.ErrInvalidModel
			}
			out = append(out, item.Interface())
			continue
		}
		if item.Kind() != reflect.Struct {
			return nil, dormerrors.ErrInvalidModel
		}
		if item.CanAddr() {
			out = append(out, item.Addr().Interface())
			continue
		}
		ptr := reflect.New(item.Type())
		ptr.Elem().Set(item)
		out = append(out, ptr.Interface())
	}
	return out, nil
}
