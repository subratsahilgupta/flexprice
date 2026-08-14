package saml

import (
	"strings"
	"testing"

	"github.com/flexprice/flexprice/internal/types"
)

// A self-signed certificate, generated for tests only. Never used outside them.
const testCertPEM = `-----BEGIN CERTIFICATE-----
MIIDUTCCAjmgAwIBAgIUajlTeNfQEKvyxPMs1o6B3MPIjngwDQYJKoZIhvcNAQEL
BQAwODEcMBoGA1UECgwTRmxleHByaWNlIFNBTUwgVGVzdDEYMBYGA1UEAwwPaWRw
LmV4YW1wbGUuY29tMB4XDTI2MDgxMjE1MjIxNFoXDTM2MDgwOTE1MjIxNFowODEc
MBoGA1UECgwTRmxleHByaWNlIFNBTUwgVGVzdDEYMBYGA1UEAwwPaWRwLmV4YW1w
bGUuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA9EppJAEYTsOY
mrj+ZLQFEPZUsw9J4K70dClbcQuTsXyJWivmaQp8nVVj1BQaPfwGqeV2sarRqaGu
CMf9dOTb/vYc+M3B9YDdzIPgIiWSvY1PmcTWErcmQBJolFqC+p1b3qKXtlt4sLXS
VXd3zyP353IHKzdxuW4dWr19m0SXCzrJEwLcd93hthz+YMlcoAb0uICAV8QC9H20
MHNEQZYeCtzm230P58kvcxPLJy1vAn/RJNbxrg2Cfu/3v0xdam6pJvW+mU/ldEPa
ywiy1WD8P3C0kbbqZ3yIxlQsS2OCZx7RjTsuTXmJMa+c7ZkUorG4JnPA4Ep/w1Cp
MAy57QfwHwIDAQABo1MwUTAdBgNVHQ4EFgQUjNSLgZR9vVZruMI1uArDrV5gXwEw
HwYDVR0jBBgwFoAUjNSLgZR9vVZruMI1uArDrV5gXwEwDwYDVR0TAQH/BAUwAwEB
/zANBgkqhkiG9w0BAQsFAAOCAQEAwmUmHHouLNqcciBxE9Gxh7LiZhRbCXj7GnYB
SXA7SsB6t2S4gbUsEhayhS4ox9Gkj2YdWr3qX7Bki+xjQKXIw6+R4E+Y6frdRbGs
I4i+zJ8u4XsCIAA2zGYtWreozUymhfmxmUnAgfzs2626yGoyY4fiZAgro/ZzP5Gw
uoZEui3jI8z/+5G8RZ9JKRAQUXKV0fhWgiE8j/WuAYNr0QTyzYEZwsB+fT8ugPiQ
2n+k84xiyB9DZJ3zm4U+aWWzpATtAxlqROLzXpFvCy/eAdphR4pfKKCOc1UE1KWF
VuExJ38hE2lsaHRIkX4oGcu5SE34zlhW7w+nZo6+zYzN542fSw==
-----END CERTIFICATE-----`

func TestConfigValidateAllowsDisabled(t *testing.T) {
	// A disabled config is the default state and must never block a settings
	// write, otherwise a tenant cannot save partial progress while onboarding.
	var cfg Config
	if err := cfg.Validate(); err != nil {
		t.Errorf("a disabled config must validate: %v", err)
	}
}

// TestConfigValidateRejectsIncompleteEnabled covers the failure this guards
// against: a half-configured provider that is switched on fails at assertion
// time, after the user has already authenticated with their IdP — a confusing
// place to discover a typo.
func TestConfigValidateRejectsIncompleteEnabled(t *testing.T) {
	base := func() Config {
		return Config{
			Enabled:        true,
			IDPEntityID:    "https://idp.example.com/metadata",
			IDPSSOUrl:      "https://idp.example.com/sso",
			IDPCertificate: testCertPEM,
			DefaultRole:    string(types.RoleAllReader),
		}
	}

	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"missing entity id", func(c *Config) { c.IDPEntityID = "" }, "idp_entity_id"},
		{"missing sso url", func(c *Config) { c.IDPSSOUrl = "" }, "idp_sso_url"},
		{"missing certificate", func(c *Config) { c.IDPCertificate = "" }, "idp_certificate"},
		{"missing default role", func(c *Config) { c.DefaultRole = "" }, "default_role"},
		{"plaintext sso url", func(c *Config) { c.IDPSSOUrl = "http://idp.example.com/sso" }, "https"},
		{"relative sso url", func(c *Config) { c.IDPSSOUrl = "/sso" }, "absolute"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected rejection mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// TestDefaultRoleMustBeAssignable stops a configuration change from minting a
// role the RBAC model does not grant to humans — e.g. a service-account-only
// role, or a typo that would otherwise be stored verbatim on the user row.
func TestDefaultRoleMustBeAssignable(t *testing.T) {
	for _, role := range types.UserTypeUser.AllowedRoles() {
		// super_admin is assignable to a user but not by provisioning; it has
		// its own test below.
		if role == types.RoleSuperAdmin {
			continue
		}
		if err := validateDefaultRole(string(role)); err != nil {
			t.Errorf("role %q should be assignable to a user: %v", role, err)
		}
	}

	for _, role := range []string{"event_ingestor", "root", "", "Reader"} {
		if err := validateDefaultRole(role); err == nil {
			t.Errorf("role %q must not be assignable to a user", role)
		}
	}
}

// TestDefaultRoleRejectsSuperAdmin covers the escalation path: provisioning
// happens on the strength of an assertion alone, so a default of super_admin
// would hand tenant administration to whoever the identity provider lets
// through. An administrator is promoted deliberately after their first login.
func TestDefaultRoleRejectsSuperAdmin(t *testing.T) {
	if err := validateDefaultRole(string(types.RoleSuperAdmin)); err == nil {
		t.Fatal("super_admin must not be grantable by just-in-time provisioning")
	}

	// The rule has to hold through the stored-config path too, since that is
	// what an API write actually goes through.
	value := defaultConfig()
	value["enabled"] = true
	value["idp_entity_id"] = "https://idp.example.com/metadata"
	value["idp_sso_url"] = "https://idp.example.com/sso"
	value["idp_certificate"] = testCertPEM
	value["default_role"] = string(types.RoleSuperAdmin)

	if err := validateStoredConfig(value); err == nil {
		t.Fatal("a stored config defaulting to super_admin must be rejected at write time")
	}
}

func TestConfigFromMapRoundTrip(t *testing.T) {
	cfg, err := configFromMap(defaultConfig())
	if err != nil {
		t.Fatalf("default config must decode: %v", err)
	}
	if cfg.Enabled {
		t.Error("SAML must default to disabled")
	}
	if cfg.DefaultRole != string(types.RoleAllReader) {
		t.Errorf("default role = %q, want reader — a permissive default would over-grant JIT users", cfg.DefaultRole)
	}
}

// TestLoopbackIdPAllowsPlainHTTP covers local development. An assertion travels
// through the browser, so a plaintext identity provider would expose it in
// transit — except on a loopback host, which never leaves the machine. Local
// identity providers such as SimpleSAMLphp serve plain http, and requiring
// https there makes the documented local test impossible to run.
func TestLoopbackIdPAllowsPlainHTTP(t *testing.T) {
	base := Config{
		Enabled:        true,
		IDPEntityID:    "http://localhost:8081/simplesaml/saml2/idp/metadata.php",
		IDPCertificate: testCertPEM,
		DefaultRole:    string(types.RoleAllReader),
	}

	for _, host := range []string{"localhost", "127.0.0.1"} {
		cfg := base
		cfg.IDPSSOUrl = "http://" + host + ":8081/sso"
		if err := cfg.Validate(); err != nil {
			t.Errorf("plain http on %s must be allowed for local testing: %v", host, err)
		}
	}

	// Anything non-loopback still requires https.
	cfg := base
	cfg.IDPSSOUrl = "http://idp.example.com/sso"
	if err := cfg.Validate(); err == nil {
		t.Error("plain http to a remote identity provider must be rejected")
	}
}
