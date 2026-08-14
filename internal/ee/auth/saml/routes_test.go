package saml

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/crewjam/saml"
	"github.com/gin-gonic/gin"

	"github.com/flexprice/flexprice/internal/config"
)

// newTestProvider builds a provider with the deployment-level settings the
// endpoints depend on, without needing the full server graph.
func newTestProvider(baseURL string) *samlProvider {
	cfg := &config.Configuration{}
	cfg.Auth.SAML.BaseURL = baseURL
	cfg.Auth.SAML.DashboardURL = "http://localhost:3000/auth/callback"
	return newSAMLProvider(cfg)
}

func enabledConfig() Config {
	return Config{
		Enabled:        true,
		IDPEntityID:    "http://idp.example.com/metadata",
		IDPSSOUrl:      "https://idp.example.com/sso",
		IDPCertificate: testCertPEM,
		DefaultRole:    "all_reader",
	}
}

// TestMetadataEndpointServesValidSPMetadata exercises what a customer does
// first: fetch our metadata and upload it into their identity provider. If the
// entity ID or ACS URL is wrong here, every later assertion is rejected for
// audience mismatch, which is a confusing way to discover a configuration typo.
func TestMetadataEndpointServesValidSPMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := newTestProvider("http://localhost:8080")
	sp, err := provider.serviceProvider("tenant_abc", enabledConfig())
	if err != nil {
		t.Fatalf("serviceProvider: %v", err)
	}

	router := gin.New()
	router.GET("/v1/auth/saml/:tenant/metadata", func(c *gin.Context) {
		out, err := marshalMetadata(sp)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "application/samlmetadata+xml", out)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/saml/tenant_abc/metadata", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "samlmetadata+xml") {
		t.Errorf("content type = %q, want SAML metadata", ct)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"EntityDescriptor",
		"AssertionConsumerService",
		"http://localhost:8080/v1/auth/saml/tenant_abc/acs",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metadata is missing %q", want)
		}
	}
}

// TestLoginRedirectsToIdentityProvider checks the SP-initiated entry point: the
// browser must be sent to the configured SSO URL carrying a SAMLRequest.
func TestLoginRedirectsToIdentityProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := newTestProvider("http://localhost:8080")
	cfg := enabledConfig()
	sp, err := provider.serviceProvider("tenant_abc", cfg)
	if err != nil {
		t.Fatalf("serviceProvider: %v", err)
	}

	authnRequest, err := sp.MakeAuthenticationRequest(
		cfg.IDPSSOUrl,
		"urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect",
		"urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST",
	)
	if err != nil {
		t.Fatalf("MakeAuthenticationRequest: %v", err)
	}

	redirectURL, err := authnRequest.Redirect("", sp)
	if err != nil {
		t.Fatalf("Redirect: %v", err)
	}

	if got := redirectURL.Host; got != "idp.example.com" {
		t.Errorf("redirect host = %q, want the configured identity provider", got)
	}
	if redirectURL.Query().Get("SAMLRequest") == "" {
		t.Error("redirect carries no SAMLRequest — the identity provider has nothing to answer")
	}
}

// TestRequestTrackerClaimRetiresTheAnsweredID covers the replay defence.
//
// This is a regression test for a real failure: claim() previously returned the
// outstanding IDs without removing the answered one, so posting the same
// assertion twice produced two successful logins. Live testing against
// SimpleSAMLphp caught it — the second post returned 302 instead of 403.
func TestRequestTrackerClaimRetiresTheAnsweredID(t *testing.T) {
	tr := newRequestTracker()
	tr.remember("id-1", time.Now().Add(authnRequestTTL))

	ids := tr.claim("id-1")
	if len(ids) != 1 || ids[0] != "id-1" {
		t.Fatalf("first claim = %v, want [id-1] so the assertion validates", ids)
	}

	if ids := tr.claim("id-1"); len(ids) != 0 {
		t.Errorf("second claim = %v, want empty — the replayed assertion would validate again", ids)
	}
}

// TestRequestTrackerClaimKeepsOtherRequests makes sure retiring one request does
// not disturb another browser's outstanding login.
func TestRequestTrackerClaimKeepsOtherRequests(t *testing.T) {
	tr := newRequestTracker()
	tr.remember("id-1", time.Now().Add(authnRequestTTL))
	tr.remember("id-2", time.Now().Add(authnRequestTTL))

	tr.claim("id-1")

	ids := tr.claim("")
	if len(ids) != 1 || ids[0] != "id-2" {
		t.Errorf("after retiring id-1, outstanding = %v, want [id-2]", ids)
	}
}

// TestRequestTrackerExpires stops an outstanding request from being accepted
// forever, which would leave the replay window open indefinitely.
func TestRequestTrackerExpires(t *testing.T) {
	tr := newRequestTracker()

	tr.remember("stale", time.Now().Add(-time.Minute))
	if ids := tr.claim(""); len(ids) != 0 {
		t.Errorf("expired request ID still offered: %v", ids)
	}
}

// TestDashboardRedirectCarriesToken checks the browser hand-back: the token has
// to reach the dashboard, and a missing configuration must not produce a
// redirect to an attacker-controlled default.
func TestDashboardRedirectCarriesToken(t *testing.T) {
	cfg := &config.Configuration{}
	cfg.Auth.SAML.DashboardURL = "http://localhost:3000/auth/callback"

	got := dashboardRedirect(cfg, "the-token")
	if !strings.HasPrefix(got, "http://localhost:3000/auth/callback?") {
		t.Errorf("redirect = %q, want the configured dashboard URL", got)
	}
	if !strings.Contains(got, "token=the-token") {
		t.Errorf("redirect = %q, want it to carry the token", got)
	}

	empty := &config.Configuration{}
	if got := dashboardRedirect(empty, "t"); !strings.HasPrefix(got, "/") {
		t.Errorf("unconfigured redirect = %q, want a relative path rather than an absolute URL", got)
	}
}

// marshalMetadata mirrors what the metadata handler serves.
func marshalMetadata(sp *saml.ServiceProvider) ([]byte, error) {
	return xml.MarshalIndent(sp.Metadata(), "", "  ")
}

// TestMetadataWorksBeforeConfiguration pins the onboarding order. The customer's
// administrator needs our ACS URL and entity ID to create the application in
// their identity provider, and only then can they hand back the entity ID, SSO
// URL, and certificate. Gating metadata on a configured provider deadlocks
// onboarding, so this must keep working with no configuration at all.
func TestMetadataWorksBeforeConfiguration(t *testing.T) {
	provider := newTestProvider("https://billing.acme.com")

	sp, err := provider.metadataOnlyServiceProvider("00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("metadata must be available before configuration: %v", err)
	}

	out, err := marshalMetadata(sp)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	body := string(out)
	for _, want := range []string{
		"https://billing.acme.com/v1/auth/saml/00000000-0000-0000-0000-000000000000/acs",
		"https://billing.acme.com/v1/auth/saml/00000000-0000-0000-0000-000000000000/metadata",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metadata missing %q — the identity provider would be configured to call the wrong endpoint", want)
		}
	}
}

// TestDefaultTenantAlias covers the single-tenant convenience: a self-hosted
// install has one tenant whose ID is identical everywhere, so requiring it in
// the URL means hand-copying a UUID into an identity provider.
func TestDefaultTenantAlias(t *testing.T) {
	const onPremTenant = "00000000-0000-0000-0000-000000000000"

	configured := newTestProvider("https://billing.acme.com")
	configured.cfg.Auth.SAML.DefaultTenantID = onPremTenant

	if got := configured.resolveTenant("default"); got != onPremTenant {
		t.Errorf("resolveTenant(default) = %q, want the configured tenant", got)
	}

	// An explicit ID is never rewritten, so a multi-tenant deployment behaves
	// exactly as before.
	if got := configured.resolveTenant("tenant_xyz"); got != "tenant_xyz" {
		t.Errorf("resolveTenant(tenant_xyz) = %q, want it passed through", got)
	}

	// Unconfigured, the alias resolves to nothing rather than silently picking
	// a tenant — the handler then rejects the request.
	unconfigured := newTestProvider("https://billing.acme.com")
	if got := unconfigured.resolveTenant("default"); got != "" {
		t.Errorf("unconfigured alias = %q, want empty so the request is rejected", got)
	}
}

// TestAliasAndUUIDProduceIdenticalMetadata is the trap worth guarding. If the
// alias produced metadata under the literal string "default" while the ACS
// endpoint validated against the resolved UUID, every assertion would fail
// audience validation — and the error would point at signatures, not at the
// URL. Both paths must yield byte-identical metadata.
func TestAliasAndUUIDProduceIdenticalMetadata(t *testing.T) {
	const onPremTenant = "00000000-0000-0000-0000-000000000000"

	provider := newTestProvider("https://billing.acme.com")
	provider.cfg.Auth.SAML.DefaultTenantID = onPremTenant

	viaAlias, err := provider.metadataOnlyServiceProvider(provider.resolveTenant("default"))
	if err != nil {
		t.Fatalf("alias path: %v", err)
	}
	viaUUID, err := provider.metadataOnlyServiceProvider(provider.resolveTenant(onPremTenant))
	if err != nil {
		t.Fatalf("uuid path: %v", err)
	}

	// Compare identity, not the whole document: Metadata() stamps a validUntil
	// from the clock, so two calls always differ by a few milliseconds.
	if viaAlias.EntityID != viaUUID.EntityID {
		t.Errorf("entity ID differs between alias and UUID paths: %q vs %q — assertions would fail audience validation",
			viaAlias.EntityID, viaUUID.EntityID)
	}
	if viaAlias.AcsURL.String() != viaUUID.AcsURL.String() {
		t.Errorf("ACS URL differs: %q vs %q — the identity provider would post to the wrong endpoint",
			viaAlias.AcsURL.String(), viaUUID.AcsURL.String())
	}

	aliasXML, err := marshalMetadata(viaAlias)
	if err != nil {
		t.Fatalf("marshal alias metadata: %v", err)
	}
	if !strings.Contains(string(aliasXML), onPremTenant) {
		t.Error("metadata does not carry the resolved tenant ID")
	}
	if strings.Contains(string(aliasXML), "/default/") {
		t.Error("metadata leaks the alias instead of the resolved tenant ID")
	}
}
