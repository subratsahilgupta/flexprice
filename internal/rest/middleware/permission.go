package middleware

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/rbac"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

// PermissionMiddleware handles RBAC permission checks
type PermissionMiddleware struct {
	rbacService *rbac.RBACService
	logger      *logger.Logger
}

// NewPermissionMiddleware creates a new permission middleware instance
func NewPermissionMiddleware(rbacService *rbac.RBACService, logger *logger.Logger) *PermissionMiddleware {
	return &PermissionMiddleware{
		rbacService: rbacService,
		logger:      logger,
	}
}

// RequirePermission returns a middleware that enforces two access controls:
// suspended tenants are blocked from write operations regardless of caller type,
// and service accounts are subject to RBAC role checks for the given entity and action.
func (pm *PermissionMiddleware) RequirePermission(entity types.Entity, action types.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Suspended tenants are blocked from all write operations.
		if action == types.ActionWrite && types.GetTenantInternalStatus(ctx) == types.TenantInternalStatusSuspended {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "tenant account is suspended",
			})
			return
		}

		roles := types.GetRoles(ctx)
		if !pm.rbacService.HasPermission(roles, string(entity), string(action)) {
			// A caller carrying no roles at all is refused every check, which
			// looks identical to holding the wrong role. Call it out separately:
			// it usually means the principal's roles column is null or empty
			// rather than that the role set is misconfigured.
			if len(roles) == 0 {
				pm.logger.Info(ctx, "access denied: caller has no roles assigned",
					"user_id", types.GetUserID(ctx),
					"tenant_id", types.GetTenantID(ctx),
					"path", c.Request.URL.Path,
				)
			}

			pm.logger.Info(ctx, "access denied due to insufficient RBAC roles",
				"user_id", types.GetUserID(ctx),
				"tenant_id", types.GetTenantID(ctx),
				"environment_id", types.GetEnvironmentID(ctx),
				"roles", roles,
				"entity", entity,
				"action", action,
				"path", c.Request.URL.Path,
			)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "insufficient permissions",
			})
			return
		}

		c.Next()
	}
}
