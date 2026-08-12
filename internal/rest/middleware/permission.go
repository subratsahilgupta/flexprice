package middleware

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/rbac"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
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

// RequireSuperAdmin gates administrative routes such as changing another user's
// roles. On top of the usual entity/action check it demands that the caller is a
// person holding super_admin, so a service account can never administer the
// people in a tenant no matter which roles its key was minted with.
//
// The caller kind is tested with IsServiceAccount rather than by requiring
// "user": a JWT session carries no user type at all (see setContextValues in
// auth.go), so an equality check would reject every dashboard user.
func (pm *PermissionMiddleware) RequireSuperAdmin(entity types.Entity, action types.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		if types.IsServiceAccount(ctx) || !lo.Contains(types.GetRoles(ctx), types.RoleSuperAdmin.String()) {
			pm.logger.Info(ctx, "access denied: administrative action requires a super_admin user",
				"user_id", types.GetUserID(ctx),
				"tenant_id", types.GetTenantID(ctx),
				"environment_id", types.GetEnvironmentID(ctx),
				"roles", types.GetRoles(ctx),
				"is_service_account", types.IsServiceAccount(ctx),
				"entity", entity,
				"action", action,
				"path", c.Request.URL.Path,
			)
			// Names both requirements, so it reads correctly for either half of
			// the guard: a service account fails on "user account" even when its
			// key carries super_admin, and a plain user fails on "Super Admin
			// access". Deliberately not "insufficient permissions", which
			// RequirePermission already returns for an ordinary RBAC miss.
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "This action requires a user account with Super Admin access",
			})
			return
		}

		// Falls through to the standard check so suspended tenants and the RBAC
		// rules stay enforced on this route too.
		pm.RequirePermission(entity, action)(c)
	}
}
