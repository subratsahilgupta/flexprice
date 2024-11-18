package testutil

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/google/uuid"
)

func SetupContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.CtxTenantID, types.DefaultTenantID)
	ctx = context.WithValue(ctx, types.CtxUserID, types.DefaultUserID)
	ctx = context.WithValue(ctx, types.CtxRequestID, uuid.New().String())
	return ctx
}
