package saml

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"net/url"
	"time"

	"github.com/crewjam/saml"
	"github.com/gin-gonic/gin"

	"github.com/flexprice/flexprice/internal/cache"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/ee/service"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
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

// requestTracker records the AuthnRequest IDs this deployment issued, so an
// assertion can be checked against a request we actually made.
//
// Without that check an attacker holding any valid assertion for our service
// provider can post it at the ACS endpoint and be logged in; with it, an
// assertion is usable exactly once.
//
// State lives in Redis rather than in process memory. The redirect that starts a
// login and the callback that finishes it are separate requests, and a load
// balancer is free to send them to different replicas: with the IDs held in one
// process, roughly (N-1)/N of logins would fail on an N-replica deployment, at
// random, looking like an identity provider fault. Redis is therefore required
// for SAML, which is enforced at boot rather than discovered here.
type requestTracker struct {
	cache cache.RedisCache
}

func newRequestTracker(c cache.RedisCache) *requestTracker {
	return &requestTracker{cache: c}
}

// trackerKeyPrefix namespaces outstanding requests. The tenant is part of the
// key so one tenant's request can never satisfy another's assertion.
const trackerKeyPrefix = "saml:authn_request:v1:"

func trackerKey(tenantID, id string) string {
	return trackerKeyPrefix + tenantID + ":" + id
}

// remember records an outstanding request. It expires on its own, so an
// abandoned login leaves nothing behind and the replay window stays bounded.
func (t *requestTracker) remember(ctx context.Context, tenantID, id string) {
	t.cache.Set(ctx, trackerKey(tenantID, id), true, authnRequestTTL)
}

// claim consumes the request an assertion answers, reporting whether it was
// outstanding.
//
// The delete is what makes this single-use: the ID is gone before validation
// runs, so a second post of the same assertion finds nothing to consume. It
// cannot be deferred until after validation — two concurrent replays would then
// both observe the ID as outstanding and both succeed.
//
// The returned slice is what crewjam validates InResponseTo against. It holds
// only the answered ID: an assertion answering anything else is rejected, which
// is the same outcome as the previous behaviour of returning every outstanding
// ID, without needing to enumerate them.
func (t *requestTracker) claim(ctx context.Context, tenantID, inResponseTo string) ([]string, bool) {
	if inResponseTo == "" {
		return nil, false
	}

	key := trackerKey(tenantID, inResponseTo)
	if _, found := t.cache.Get(ctx, key); !found {
		return nil, false
	}
	t.cache.Delete(ctx, key)

	return []string{inResponseTo}, true
}

// restore puts back a request that claim consumed for a login that then failed
// validation.
//
// Without it any rejected response burns the outstanding request, so an identity
// provider retrying after a transient failure — or a browser reposting once a
// clock skew resolves — has nothing left to answer and the login fails
// permanently. Only called after validation has failed, so a consumed assertion
// is never resurrected.
//
// The full TTL is granted again rather than the remainder. The window this
// reopens is bounded by the same ten minutes, and tracking the original expiry
// across replicas would cost more than it protects.
func (t *requestTracker) restore(ctx context.Context, tenantID, id string) {
	if id == "" {
		return
	}
	t.cache.Set(ctx, trackerKey(tenantID, id), true, authnRequestTTL)
}

// Handler serves the SAML browser-flow endpoints.
type Handler struct {
	cfg           *config.Configuration
	serviceParams service.ServiceParams
	logger        *logger.Logger
	tracker       *requestTracker
}

func NewHandler(cfg *config.Configuration, serviceParams service.ServiceParams, logger *logger.Logger) *Handler {
	return &Handler{
		cfg:           cfg,
		serviceParams: serviceParams,
		logger:        logger,
		tracker:       newRequestTracker(serviceParams.RedisCache),
	}
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

	// Unchecked because these endpoints run before a session exists: there is no
	// caller to authorise, and the browser arriving here is precisely the one
	// trying to establish who it is. What the configuration permits is decided
	// below, from the configuration itself.
	resp, err := settingsSvc.GetSettingByKeyUnchecked(ctx, SettingKeySAML)
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

	h.tracker.remember(c.Request.Context(), tenantID, authnRequest.ID)

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
	answeredID := inResponseTo(c.Request)
	possibleIDs, consumed := h.tracker.claim(c.Request.Context(), tenantID, answeredID)

	result, err := validateAssertion(sp, c.Request, possibleIDs, cfg)
	if err != nil {
		// Put the request back. It was consumed before validation so a
		// concurrent replay could not also observe it, but this response never
		// became a login: leaving it consumed would mean an identity provider
		// retrying after a transient failure, or a browser reposting once a
		// clock skew resolves, has nothing left to answer.
		if consumed {
			h.tracker.restore(c.Request.Context(), tenantID, answeredID)
		}

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

	c.Redirect(http.StatusFound, dashboardRedirect(h.cfg, tenantID, token))
}

// dashboardRedirect hands the token back to the browser application.
//
// The token travels in the URL fragment rather than the query string. A
// fragment is never sent to a server: it stays out of access logs at every
// proxy and CDN in the path, and out of the Referer header of every request the
// dashboard makes once it loads. A query parameter reaches all of those, and
// this token is valid for thirty days, so each of them is a durable copy of a
// live credential.
//
// The fragment is still visible in browser history and to script on the page,
// which is why the dashboard clears it from the URL as soon as it has read it.
// Removing that exposure altogether needs a single-use code exchanged for the
// token, or an HttpOnly cookie set by this handler — both change the contract
// with the dashboard and are worth doing deliberately rather than here.
// The tenant travels with it so the dashboard can confirm the callback answers
// the login that browser started. It is not trusted as an identity — the
// dashboard compares it against the tenant it recorded when it began the flow
// and refuses a mismatch, which is what stops a token minted for one tenant
// being planted on a browser mid-login to another.
func dashboardRedirect(cfg *config.Configuration, tenantID, token string) string {
	base := cfg.Auth.SAML.DashboardURL
	if base == "" {
		base = "/"
	}
	u, err := url.Parse(base)
	if err != nil {
		return "/"
	}

	fragment := url.Values{}
	fragment.Set("token", token)
	fragment.Set("tenant_id", tenantID)
	u.Fragment = fragment.Encode()
	return u.String()
}

// inResponseTo extracts the AuthnRequest ID an assertion claims to answer,
// without validating anything — the value is used only to retire the matching
// outstanding request.
//
// Parsed as XML rather than matched with a pattern. The value decides which
// outstanding request is retired, so failing to find it silently disables the
// replay defence: claim("") retires nothing and the same assertion can be
// posted again until the request expires. A pattern would have to agree with
// every identity provider's serialisation to avoid that — single-quoted
// attributes and whitespace around "=" are both valid XML — and it would also
// have to avoid matching InResponseTo on nested elements such as
// SubjectConfirmationData, which appears before the Response attribute in some
// documents and would retire the wrong ID.
//
// Only the root element's attribute is read; the decoder stops at the first
// token, so a malformed or hostile document costs nothing to reject.
func inResponseTo(r *http.Request) string {
	raw := r.PostFormValue("SAMLResponse")
	if raw == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return ""
	}

	decoder := xml.NewDecoder(bytes.NewReader(decoded))
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			// Skip the XML declaration, comments and whitespace preceding the
			// root element.
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Local == "InResponseTo" {
				return attr.Value
			}
		}
		return ""
	}
}
