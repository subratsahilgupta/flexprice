package saml

import (
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/crewjam/saml"
	"github.com/gin-gonic/gin"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/ee/service"
	"github.com/flexprice/flexprice/internal/logger"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

const (
	// authnRequestTTL bounds how long an outstanding AuthnRequest is accepted.
	// Short enough to limit the replay window, long enough for a user to type a
	// password and clear MFA at their identity provider.
	authnRequestTTL = 10 * time.Minute

	// tokenExpiryHours matches the community login token lifetime.
	tokenExpiryHours = 24 * 30
)

// requestTracker remembers the AuthnRequest IDs this deployment issued.
//
// An assertion must answer a request we made: without that check an attacker who
// obtains any valid assertion for our service provider can replay it at the ACS
// endpoint. Entries expire with the request they describe.
//
// This is process-local, which means a deployment running several API replicas
// behind a load balancer can bounce a user whose callback lands on a different
// replica. Moving it to Redis is the obvious next step; it is called out in the
// handler so the limitation is not discovered in production.
type requestTracker struct {
	mu  sync.Mutex
	ids map[string]time.Time
}

func newRequestTracker() *requestTracker {
	return &requestTracker{ids: map[string]time.Time{}}
}

func (t *requestTracker) remember(id string, expiry time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.evictExpiredLocked()
	t.ids[id] = expiry
}

// claim atomically takes the outstanding request IDs and removes the one the
// assertion answers, so a second post of the same assertion finds nothing to
// match and is rejected.
//
// Taking and removing under one lock is what makes this a replay defence rather
// than a hint: two concurrent posts of the same assertion cannot both observe
// the ID as outstanding.
func (t *requestTracker) claim(inResponseTo string) []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.evictExpiredLocked()

	out := make([]string, 0, len(t.ids))
	for id := range t.ids {
		out = append(out, id)
	}

	// Remove the answered ID. An assertion carrying an unknown InResponseTo is
	// left for crewjam/saml to reject against the list we just returned.
	if inResponseTo != "" {
		delete(t.ids, inResponseTo)
	}
	return out
}

func (t *requestTracker) evictExpiredLocked() {
	now := time.Now()
	for id, exp := range t.ids {
		if now.After(exp) {
			delete(t.ids, id)
		}
	}
}

var tracker = newRequestTracker()

// Handler serves the SAML browser-flow endpoints.
type Handler struct {
	cfg           *config.Configuration
	serviceParams service.ServiceParams
	logger        *logger.Logger
}

func NewHandler(cfg *config.Configuration, serviceParams service.ServiceParams, logger *logger.Logger) *Handler {
	return &Handler{cfg: cfg, serviceParams: serviceParams, logger: logger}
}

// RegisterRoutes mounts the SAML endpoints on the public group. They run before
// a session exists: the login redirect starts the flow, and the ACS callback is
// posted by the identity provider, which carries no Flexprice credentials.
//
// A deployment with SAML switched off mounts nothing, so the endpoints 404 as
// though the feature did not exist rather than announcing a disabled one.
func (h *Handler) RegisterRoutes(public *gin.RouterGroup) {
	if !h.cfg.Auth.SAML.Enabled {
		return
	}

	group := public.Group("/auth/saml/:tenant")
	{
		group.GET("/metadata", h.Metadata)
		group.GET("/login", h.Login)
		group.POST("/acs", h.ACS)
	}
}


// tenantConfig loads a tenant's identity provider configuration.
//
// The tenant comes from the URL rather than a session, so it is attacker-chosen.
// That is safe because the configuration itself decides everything that follows:
// an unconfigured or disabled tenant yields no provider, and the assertion is
// verified against that tenant's certificate.
func (h *Handler) tenantConfig(c *gin.Context) (string, Config, error) {
	// The resolved ID is used for everything downstream — the settings lookup,
	// the SP entity ID, and the ACS URL — so metadata and assertion validation
	// always agree on who we are. Resolving in one place is what keeps them
	// consistent.
	tenantID := newSAMLProvider(h.cfg).resolveTenant(c.Param("tenant"))
	if tenantID == "" {
		return "", Config{}, ierr.NewError("tenant is required").
			WithHint("Use the tenant ID in the path: /v1/auth/saml/{tenant_id}/...").
			Mark(ierr.ErrValidation)
	}

	ctx := types.SetTenantID(c.Request.Context(), tenantID)
	settingsSvc := service.NewSettingsService(h.serviceParams)

	resp, err := settingsSvc.GetSettingByKey(ctx, SettingKeySAML)
	if err != nil {
		return "", Config{}, err
	}

	cfg, err := configFromMap(resp.Value)
	if err != nil {
		return "", Config{}, err
	}
	// Both the tenant's switch and the Flexprice-side approval must be on. The
	// two are reported identically: whether a tenant is unapproved rather than
	// unconfigured is not something an unauthenticated caller should learn.
	if !cfg.IsLive() {
		return "", Config{}, ierr.NewError("saml is not enabled for this organisation").
			Mark(ierr.ErrNotFound)
	}
	return tenantID, cfg, nil
}

// metadataHandler serves SP metadata for the customer to upload into their
// identity provider. It is the first endpoint a customer needs, and works before
// any login has ever succeeded.
func (h *Handler) Metadata(c *gin.Context) {
	provider := newSAMLProvider(h.cfg)
	tenantID := provider.resolveTenant(c.Param("tenant"))
	if tenantID == "" {
		c.Error(ierr.NewError("tenant is required").
			WithHint("Use the tenant ID in the path: /v1/auth/saml/{tenant_id}/...").
			Mark(ierr.ErrValidation))
		return
	}

	// Deliberately not gated on a configured identity provider. Onboarding
	// runs the other way round: the customer's administrator needs our ACS
	// URL and entity ID to create the application in their identity
	// provider, and only then can they hand back the entity ID, SSO URL,
	// and certificate we would be validating here. Requiring configuration
	// first is a deadlock.
	//
	// Nothing sensitive is exposed: this metadata is derived entirely from
	// base_url and the tenant in the path, both of which the caller already
	// knows.
	sp, err := provider.metadataOnlyServiceProvider(tenantID)
	if err != nil {
		c.Error(err)
		return
	}

	out, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		c.Error(err)
		return
	}
	c.Data(http.StatusOK, "application/samlmetadata+xml", out)
}

// Login starts SP-initiated login by redirecting to the identity provider with
// a signed AuthnRequest.
func (h *Handler) Login(c *gin.Context) {
	tenantID, cfg, err := h.tenantConfig(c)
	if err != nil {
		c.Error(err)
		return
	}

	provider := newSAMLProvider(h.cfg)
	sp, err := provider.serviceProvider(tenantID, cfg)
	if err != nil {
		c.Error(err)
		return
	}

	authnRequest, err := sp.MakeAuthenticationRequest(
		sp.GetSSOBindingLocation(saml.HTTPRedirectBinding),
		saml.HTTPRedirectBinding,
		saml.HTTPPostBinding,
	)
	if err != nil {
		c.Error(err)
		return
	}

	tracker.remember(authnRequest.ID, time.Now().Add(authnRequestTTL))

	redirectURL, err := authnRequest.Redirect("", sp)
	if err != nil {
		c.Error(err)
		return
	}
	c.Redirect(http.StatusFound, redirectURL.String())
}

// ACS consumes the identity provider's assertion and completes login.
func (h *Handler) ACS(c *gin.Context) {
	tenantID, cfg, err := h.tenantConfig(c)
	if err != nil {
		c.Error(err)
		return
	}

	if err := c.Request.ParseForm(); err != nil {
		c.Error(ierr.NewError("malformed saml response").Mark(ierr.ErrValidation))
		return
	}

	provider := newSAMLProvider(h.cfg)
	sp, err := provider.serviceProvider(tenantID, cfg)
	if err != nil {
		c.Error(err)
		return
	}

	// The InResponseTo is read before validation purely to know which
	// outstanding request to retire; nothing is trusted from it, since the
	// assertion is validated against the returned list immediately after.
	possibleIDs := tracker.claim(inResponseTo(c.Request))

	result, err := validateAssertion(sp, c.Request, possibleIDs, cfg)
	if err != nil {
		h.logger.Error(c.Request.Context(), "saml assertion rejected",
			"error", err, "tenant_id", tenantID)
		c.Error(err)
		return
	}

	ctx := types.SetTenantID(c.Request.Context(), tenantID)
	userID, err := provider.resolveUser(ctx, h.serviceParams, tenantID, result.Email, cfg)
	if err != nil {
		h.logger.Error(ctx, "saml login could not resolve a user",
			"error", err, "tenant_id", tenantID)
		c.Error(err)
		return
	}

	// The token is minted by the built-in provider, so its claims are
	// identical to a password login and every existing middleware validates
	// it unchanged.
	token, _, err := provider.tokens.GenerateDevToken(tenantID, "", userID, result.Email, tokenExpiryHours)
	if err != nil {
		c.Error(err)
		return
	}

	h.logger.Info(ctx, "saml login succeeded",
		"tenant_id", tenantID, "user_id", userID)

	c.Redirect(http.StatusFound, dashboardRedirect(h.cfg, token))
}

// dashboardRedirect hands the token back to the browser application.
func dashboardRedirect(cfg *config.Configuration, token string) string {
	base := cfg.Auth.SAML.DashboardURL
	if base == "" {
		base = "/"
	}
	u, err := url.Parse(base)
	if err != nil {
		return "/"
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

// inResponseTo extracts the AuthnRequest ID an assertion claims to answer,
// without validating anything — the value is used only to retire the matching
// outstanding request.
func inResponseTo(r *http.Request) string {
	raw := r.PostFormValue("SAMLResponse")
	if raw == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return ""
	}
	m := inResponseToPattern.FindSubmatch(decoded)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}

var inResponseToPattern = regexp.MustCompile(`InResponseTo="([^"]+)"`)
