package access

import "context"

type Context struct {
	UserID         any
	CompanyID      any
	TenantID       any
	OrganizationID any
	WorkspaceID    any
	WarehouseID    any
	Values         map[string]any
}

type contextKey struct{}

func WithContext(parent context.Context, value Context) context.Context {
	return context.WithValue(parent, contextKey{}, value)
}

func FromContext(ctx context.Context) (Context, bool) {
	value, ok := ctx.Value(contextKey{}).(Context)
	return value, ok
}
