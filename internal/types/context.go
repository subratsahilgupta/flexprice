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
	if userID, ok := ctx.Value(CtxUserID).(string); ok {
		return userID
	}
	return ""
}

func GetTenantID(ctx context.Context) string {
	if tenantID, ok := ctx.Value(CtxTenantID).(string); ok {
		return tenantID
	}
	return ""
}

func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(CtxRequestID).(string); ok {
		return requestID
	}
	return ""
}
