// Package saml implements SAML 2.0 single sign-on so a self-managed customer
// can authenticate dashboard users against their own identity provider.
//
// The provider does not mint its own tokens. After an assertion is validated it
// delegates to the built-in flexprice provider, so the JWT claim schema, signing
// secret, and every middleware that validates it stay exactly as they are for
// password login.
package saml

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/auth"
	"github.com/flexprice/flexprice/internal/config"
	domainauth "github.com/flexprice/flexprice/internal/domain/auth"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// ProviderName is the value operators set as auth.provider.
const ProviderName types.AuthProvider = "saml"

// samlProvider implements auth.Provider. Private per the repo style guide; the
// community build only ever holds it as an auth.Provider.
type samlProvider struct {
	cfg *config.Configuration
	// tokens is the built-in provider. Everything that is not SSO-specific —
	// token minting and validation, portal and checkout tokens — delegates to it
	// so SAML never forks the claim schema.
	tokens auth.Provider
}

// init registers this package as the SAML implementation. internal/auth cannot
// import this package (it would be a cycle), so registration runs the other way.
func init() {
	auth.RegisterSAMLProvider(func(cfg *config.Configuration) auth.Provider {
		return newSAMLProvider(cfg)
	})
}

func newSAMLProvider(cfg *config.Configuration) *samlProvider {
	return &samlProvider{cfg: cfg, tokens: auth.NewFlexpriceAuth(cfg)}
}

// baseURL is the externally reachable origin, used to build the SP entity ID and
// ACS URL that appear in our metadata.
func (p *samlProvider) baseURL() string {
	return strings.TrimSuffix(p.cfg.Auth.SAML.BaseURL, "/")
}

// TenantAlias is the path segment a single-tenant deployment may use instead of
// a tenant UUID.
const TenantAlias = "default"

// resolveTenant maps the tenant path segment to a tenant ID.
//
// A self-hosted install has exactly one tenant whose ID is identical on every
// install, so requiring it in the URL means the operator hand-copies a UUID into
// their identity provider — an easy thing to get subtly wrong, and the failure
// (audience mismatch) does not point at the cause. Configuring
// auth.saml.default_tenant_id lets those URLs read /v1/auth/saml/default/...
//
// The alias resolves only when configured. A multi-tenant deployment leaves it
// empty, and "default" is then treated as an ordinary tenant ID that will not
// match anything.
func (p *samlProvider) resolveTenant(segment string) string {
	if segment != TenantAlias {
		return segment
	}
	return p.cfg.Auth.SAML.DefaultTenantID
}

func base64DER(cert *x509.Certificate) string {
	return base64.StdEncoding.EncodeToString(cert.Raw)
}

func (p *samlProvider) GetProvider() types.AuthProvider { return ProviderName }

// Login is not password-based for SAML. The browser flow runs through the
// endpoints in routes.go and finishes at the ACS callback.
//
// Reaching this method means a user posted credentials to /auth/login while the
// tenant is configured for SSO. Whether that is allowed is a deployment
// decision, so it is gated by config rather than rejected outright.
func (p *samlProvider) Login(ctx context.Context, req auth.AuthRequest, userAuthInfo *domainauth.Auth) (*auth.AuthResponse, error) {
	if p.cfg.Auth.SAML.EnforceSSO {
		return nil, ierr.NewError("password login is disabled for this deployment").
			WithHint("Sign in through your identity provider").
			Mark(ierr.ErrPermissionDenied)
	}
	return p.tokens.Login(ctx, req, userAuthInfo)
}

// SignUp is not reachable through SSO: a tenant is created by an administrator,
// and its users arrive by just-in-time provisioning at the ACS endpoint.
func (p *samlProvider) SignUp(ctx context.Context, req auth.AuthRequest) (*auth.AuthResponse, error) {
	return p.tokens.SignUp(ctx, req)
}

// UserInvite returns no auth record, which is what makes just-in-time
// provisioning clean: the user service only writes a password row when one is
// returned, so an SSO user exists with no credential of its own.
func (p *samlProvider) UserInvite(ctx context.Context, req auth.UserInviteRequest) (*auth.UserInviteResponse, error) {
	return &auth.UserInviteResponse{}, nil
}

func (p *samlProvider) AssignUserToTenant(ctx context.Context, userID, tenantID string) error {
	return nil
}

// The remaining methods are identity-provider agnostic and delegate unchanged.

func (p *samlProvider) ValidateToken(ctx context.Context, token string) (*domainauth.Claims, error) {
	return p.tokens.ValidateToken(ctx, token)
}

func (p *samlProvider) GenerateSessionToken(customerID, externalCustomerID, tenantID, environmentID string, timeoutHours int) (string, time.Time, error) {
	return p.tokens.GenerateSessionToken(customerID, externalCustomerID, tenantID, environmentID, timeoutHours)
}

func (p *samlProvider) ValidateSessionToken(ctx context.Context, token string) (*domainauth.SessionClaims, error) {
	return p.tokens.ValidateSessionToken(ctx, token)
}

func (p *samlProvider) GenerateDevToken(tenantID, environmentID, userID, email string, expiryHours int) (string, time.Time, error) {
	return p.tokens.GenerateDevToken(tenantID, environmentID, userID, email, expiryHours)
}

func (p *samlProvider) GenerateCheckoutToken(extraClaims map[string]interface{}) (string, error) {
	return p.tokens.GenerateCheckoutToken(extraClaims)
}

