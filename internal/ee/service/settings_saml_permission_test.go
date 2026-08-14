package service

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/config"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// samlEnabledService builds the minimum service needed to exercise the gate on
// a deployment that offers SAML.
func samlEnabledService(enabled bool) *settingsService {
	cfg := &config.Configuration{}
	cfg.Auth.SAML.Enabled = enabled
	return &settingsService{ServiceParams: ServiceParams{Config: cfg}}
}

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

	svc := samlEnabledService(true)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.requireSuperAdminForAuthSetting(tc.ctx, tc.key)
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

// TestSAMLConfigRefusedWhenDeploymentDisablesSAML covers the deployment-level
// kill switch: with SAML off, no tenant may store a configuration, so the
// feature cannot look half-set-up on a deployment that serves no SAML routes.
func TestSAMLConfigRefusedWhenDeploymentDisablesSAML(t *testing.T) {
	svc := samlEnabledService(false)
	ctx := ctxWithRoles(types.UserTypeUser, types.RoleSuperAdmin.String())

	err := svc.requireSuperAdminForAuthSetting(ctx, types.SettingKeySAMLConfig)
	if err == nil {
		t.Fatal("a deployment with SAML disabled must refuse the write, even for a super admin")
	}
	// Reported as absent rather than forbidden: the feature does not exist here.
	if !ierr.IsNotFound(err) {
		t.Errorf("expected a not-found error, got %v", err)
	}

	// Other settings are untouched by the SAML switch.
	if err := svc.requireSuperAdminForAuthSetting(ctx, types.SettingKeyInvoiceConfig); err != nil {
		t.Errorf("an unrelated setting must not be gated on the SAML switch: %v", err)
	}
}

// TestActiveIsNotSettableThroughTheAPI is the test that matters for the ops
// gate: "active" is Flexprice's approval that a tenant may serve SSO, so a
// tenant able to set it in an ordinary settings write would be approving itself.
func TestActiveIsNotSettableThroughTheAPI(t *testing.T) {
	protected := apiImmutableSettingFields(types.SettingKeySAMLConfig)
	if !lo.Contains(protected, "active") {
		t.Fatalf(`"active" must be protected from API writes, got %v`, protected)
	}

	stored := map[string]interface{}{"enabled": true, "active": false, "enforce_sso": false}
	request := map[string]interface{}{"active": true, "enforce_sso": true}

	merged := mergePreservingImmutableFields(types.SettingKeySAMLConfig, stored, request)

	if merged["active"] != false {
		t.Error("a request setting active:true must not grant approval")
	}
	// The tenant's own fields still apply — only the approval is pinned.
	if merged["enforce_sso"] != true {
		t.Error("enforce_sso is the tenant's own setting and must remain writable")
	}
	if merged["enabled"] != true {
		t.Error("untouched fields must survive the merge")
	}
}

// TestMergePreservingImmutableFieldsLeavesOtherKeysAlone confirms the merge is
// unchanged for every setting that has no protected fields.
func TestMergePreservingImmutableFieldsLeavesOtherKeysAlone(t *testing.T) {
	stored := map[string]interface{}{"a": 1, "b": 2}
	request := map[string]interface{}{"b": 3, "c": 4}

	merged := mergePreservingImmutableFields(types.SettingKeyInvoiceConfig, stored, request)

	for field, want := range map[string]interface{}{"a": 1, "b": 3, "c": 4} {
		if merged[field] != want {
			t.Errorf("field %q = %v, want %v", field, merged[field], want)
		}
	}
}
