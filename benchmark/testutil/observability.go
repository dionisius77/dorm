package testutil

import (
	"context"

	"github.com/dionisius77/dorm/orm"
)

type NopTracerProvider struct{}

type NopTracer struct{}

type NopSpan struct{}

func (NopTracerProvider) Tracer(string) orm.Tracer { return NopTracer{} }

func (NopTracer) Start(ctx context.Context, _ string, _ ...orm.SpanOption) (context.Context, orm.Span) {
	return ctx, NopSpan{}
}

func (NopSpan) End() {}

func (NopSpan) RecordError(error) {}

func (NopSpan) SetAttributes(...orm.Attribute) {}
