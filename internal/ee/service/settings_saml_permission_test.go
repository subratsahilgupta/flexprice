package service

import (
	"context"
	"testing"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

func ctxWithRoles(userType types.UserType, roles ...string) context.Context {
	ctx := context.WithValue(context.Background(), types.CtxRoles, roles)
	return context.WithValue(ctx, types.CtxUserType, string(userType))
}

// TestRequireSuperAdminForAuthSetting covers the escalation path the gate
// exists to close: the shared settings route is gated on setting:write, which
// an all_writer holds, but a SAML configuration names the identity provider
// this tenant trusts.
func TestRequireSuperAdminForAuthSetting(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		key     types.SettingKey
		wantErr bool
	}{
		{
			name: "super_admin user may configure saml",
			ctx:  ctxWithRoles(types.UserTypeUser, types.RoleSuperAdmin.String()),
			key:  types.SettingKeySAMLConfig,
		},
		{
			name:    "all_writer may not configure saml",
			ctx:     ctxWithRoles(types.UserTypeUser, types.RoleAllWriter.String()),
			key:     types.SettingKeySAMLConfig,
			wantErr: true,
		},
		{
			name:    "all_reader may not configure saml",
			ctx:     ctxWithRoles(types.UserTypeUser, types.RoleAllReader.String()),
			key:     types.SettingKeySAMLConfig,
			wantErr: true,
		},
		{
			name:    "caller with no roles may not configure saml",
			ctx:     ctxWithRoles(types.UserTypeUser),
			key:     types.SettingKeySAMLConfig,
			wantErr: true,
		},
		{
			// Administering how people log in is not a machine action, so a
			// service account is refused even holding super_admin. Matches
			// middleware.SuperAdminOnly.
			name:    "service account may not configure saml even as super_admin",
			ctx:     ctxWithRoles(types.UserTypeServiceAccount, types.RoleSuperAdmin.String()),
			key:     types.SettingKeySAMLConfig,
			wantErr: true,
		},
		{
			// The gate is key-specific: every other setting keeps the broad
			// setting:write permission the route already enforces.
			name: "unrelated setting is unaffected for all_writer",
			ctx:  ctxWithRoles(types.UserTypeUser, types.RoleAllWriter.String()),
			key:  types.SettingKeyInvoiceConfig,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireSuperAdminForAuthSetting(tc.ctx, tc.key)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected the write to be refused")
				}
				// Must surface as 403, not 500 — the caller needs to know it is
				// a permission problem, not a server fault.
				if !ierr.IsPermissionDenied(err) {
					t.Errorf("expected a permission-denied error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected the write to be allowed, got %v", err)
			}
		})
	}
}
