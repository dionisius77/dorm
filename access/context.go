package access

import "context"

// Context carries request-scoped access information for policy evaluation.
type Context struct {
	UserID         any
	CompanyID      any
	TenantID       any
	OrganizationID any
	WorkspaceID    any
	WarehouseID    any
	Values         map[string]any
	Policy         Policy
}

type contextKey struct{}

// WithContext stores access metadata in a parent context.
func WithContext(parent context.Context, value Context) context.Context {
	return context.WithValue(parent, contextKey{}, value)
}

// FromContext extracts access metadata from a context.
func FromContext(ctx context.Context) (Context, bool) {
	value, ok := ctx.Value(contextKey{}).(Context)
	return value, ok
}
