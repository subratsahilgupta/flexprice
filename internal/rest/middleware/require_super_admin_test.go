package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// superAdminRolesJSON gives all_writer real write permission, so a caller
// rejected by RequireSuperAdmin is provably rejected for lacking super_admin
// rather than for failing the underlying RBAC check.
const superAdminRolesJSON = `{
	"super_admin": {
		"name": "Super Admin",
		"permissions": { "*": ["*"] }
	},
	"all_writer": {
		"name": "All Writer",
		"permissions": { "*": ["read", "write"] }
	},
	"event_ingestor": {
		"name": "Event Ingestor",
		"permissions": { "event": ["write"] }
	}
}`

// newSuperAdminRouter mirrors newRBACRouter but gates the route with
// RequireSuperAdmin instead of RequirePermission.
func newSuperAdminRouter(t *testing.T, tenantStatus types.TenantInternalStatus, userType string, roles []string) *gin.Engine {
	t.Helper()
	log := newTestLogger(t)
	permMW := NewPermissionMiddleware(newRBACServiceWithRoles(t, superAdminRolesJSON), log)

	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, types.CtxTenantID, "tenant-rbac")
		ctx = context.WithValue(ctx, types.CtxUserID, "user-1")
		if userType != "" {
			ctx = context.WithValue(ctx, types.CtxUserType, userType)
		}
		if roles != nil {
			ctx = context.WithValue(ctx, types.CtxRoles, roles)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	r.Use(TenantStatusMiddleware(&mockTenantService{status: tenantStatus}, log))
	r.PUT("/users/:id/roles", permMW.RequireSuperAdmin(types.EntityUser, types.ActionWrite), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	return r
}

func TestRequireSuperAdmin(t *testing.T) {
	testCases := []struct {
		name         string
		userType     string
		roles        []string
		tenantStatus types.TenantInternalStatus
		wantCode     int
		wantErrMsg   string
	}{
		{
			// A dashboard session carries no user type at all, so this is the
			// ordinary success path and would break under a userType == "user" check.
			name:         "super_admin JWT user with no user type is allowed",
			userType:     "",
			roles:        []string{types.RoleSuperAdmin.String()},
			tenantStatus: types.TenantInternalStatusActive,
			wantCode:     http.StatusOK,
		},
		{
			name:         "super_admin user is allowed",
			userType:     string(types.UserTypeUser),
			roles:        []string{types.RoleSuperAdmin.String()},
			tenantStatus: types.TenantInternalStatusActive,
			wantCode:     http.StatusOK,
		},
		{
			// all_writer passes the entity/action check, so only the super_admin
			// guard can be what refuses it here.
			name:         "user with write access but not super_admin is denied",
			userType:     string(types.UserTypeUser),
			roles:        []string{types.RoleAllWriter.String()},
			tenantStatus: types.TenantInternalStatusActive,
			wantCode:     http.StatusForbidden,
			wantErrMsg:   "This action requires a user account with Super Admin access",
		},
		{
			// The key carries super_admin, yet administering people stays closed
			// to machine credentials.
			name:         "service account holding super_admin is denied",
			userType:     string(types.UserTypeServiceAccount),
			roles:        []string{types.RoleSuperAdmin.String()},
			tenantStatus: types.TenantInternalStatusActive,
			wantCode:     http.StatusForbidden,
			wantErrMsg:   "This action requires a user account with Super Admin access",
		},
		{
			name:         "service account without super_admin is denied",
			userType:     string(types.UserTypeServiceAccount),
			roles:        []string{types.RoleEventIngestor.String()},
			tenantStatus: types.TenantInternalStatusActive,
			wantCode:     http.StatusForbidden,
			wantErrMsg:   "This action requires a user account with Super Admin access",
		},
		{
			name:         "caller with no roles is denied",
			userType:     string(types.UserTypeUser),
			roles:        nil,
			tenantStatus: types.TenantInternalStatusActive,
			wantCode:     http.StatusForbidden,
			wantErrMsg:   "This action requires a user account with Super Admin access",
		},
		{
			name:         "caller with an empty role set is denied",
			userType:     string(types.UserTypeUser),
			roles:        []string{},
			tenantStatus: types.TenantInternalStatusActive,
			wantCode:     http.StatusForbidden,
			wantErrMsg:   "This action requires a user account with Super Admin access",
		},
		{
			// Falling through to RequirePermission is what keeps the suspension
			// rule in force on admin routes.
			name:         "super_admin in a suspended tenant is denied",
			userType:     string(types.UserTypeUser),
			roles:        []string{types.RoleSuperAdmin.String()},
			tenantStatus: types.TenantInternalStatusSuspended,
			wantCode:     http.StatusForbidden,
			wantErrMsg:   "tenant account is suspended",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := newSuperAdminRouter(t, tc.tenantStatus, tc.userType, tc.roles)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPut, "/users/user-2/roles", nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.wantCode, w.Code)

			if tc.wantErrMsg != "" {
				var body map[string]string
				assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				assert.Equal(t, tc.wantErrMsg, body["message"])
			}
		})
	}
}
