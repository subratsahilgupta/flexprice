package types

import "context"

// ContextKey is a type for the keys of values stored in the context
type ContextKey string

const (
	CtxRequestID ContextKey = "ctx_request_id"
	CtxTenantID  ContextKey = "ctx_tenant_id"
	CtxUserID    ContextKey = "ctx_user_id"

	// Default values
	DefaultTenantID = "00000000-0000-0000-0000-000000000000"
	DefaultUserID   = "00000000-0000-0000-0000-000000000000"
)

func GetUserID(ctx context.Context) string {
	return ctx.Value(CtxUserID).(string)
}

func GetTenantID(ctx context.Context) string {
	return ctx.Value(CtxTenantID).(string)
}

func GetRequestID(ctx context.Context) string {
	return ctx.Value(CtxRequestID).(string)
}
