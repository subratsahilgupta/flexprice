package middleware

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware is a middleware that authenticates requests
// TODO: Implement authentication
// For now set the default tenant ID and user ID in the request context
func AuthMiddleware(c *gin.Context) {
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, types.CtxTenantID, types.DefaultTenantID)
	ctx = context.WithValue(ctx, types.CtxUserID, types.DefaultUserID)
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}
