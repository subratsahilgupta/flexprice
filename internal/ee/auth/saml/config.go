package saml

import (
	"fmt"
	"net/url"
	"strings"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/utils"
)

// SettingKeySAML is the tenant-level settings key holding a tenant's identity
// provider configuration.
const SettingKeySAML types.SettingKey = "saml_config"

// Config is a tenant's SAML identity provider configuration.
//
// Stored as a tenant-level setting rather than its own table: an identity
// provider is per-organisation, and tenant-level settings are readable without
// an environment in context, which the pre-login endpoints require.
//
// Only the identity provider's public signing certificate is held here, so
// plain JSONB storage is appropriate. An SP private key would need
// internal/security's EncryptionService instead.
type Config struct {
	Enabled bool `json:"enabled"`

	// IDPEntityID identifies the identity provider; it must match the Issuer of
	// every assertion we accept.
	IDPEntityID string `json:"idp_entity_id"`
	// IDPSSOUrl is where a user is redirected to authenticate.
	IDPSSOUrl string `json:"idp_sso_url"`
	// IDPCertificate is the PEM-encoded X.509 certificate whose key signs
	// assertions. Assertions signed by anything else are rejected.
	IDPCertificate string `json:"idp_certificate"`

	// EmailAttribute names the assertion attribute carrying the user's email.
	// Empty means use the NameID.
	EmailAttribute string `json:"email_attribute"`

	// DefaultRole is granted to just-in-time provisioned users. An admin adjusts
	// it afterwards.
	DefaultRole string `json:"default_role"`
}

func defaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"enabled":         false,
		"idp_entity_id":   "",
		"idp_sso_url":     "",
		"idp_certificate": "",
		"email_attribute": "",
		"default_role":    string(types.RoleAllReader),
	}
}

// Validate rejects a configuration that cannot produce a safe login.
//
// The checks are deliberately strict about enablement: a half-configured
// provider that is switched on would fail at assertion time, after the user has
// already authenticated with their identity provider, which is a confusing place
// to discover a typo.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if strings.TrimSpace(c.IDPEntityID) == "" {
		return fmt.Errorf("idp_entity_id is required when SAML is enabled")
	}

	if strings.TrimSpace(c.IDPSSOUrl) == "" {
		return fmt.Errorf("idp_sso_url is required when SAML is enabled")
	}
	u, err := url.Parse(c.IDPSSOUrl)
	if err != nil || u.Host == "" {
		return fmt.Errorf("idp_sso_url must be an absolute URL")
	}
	// An assertion travels through the browser, so a plaintext identity
	// provider exposes it in transit. The sole exemption is a loopback host,
	// which never leaves the machine and is how a local identity provider is
	// run during development — SimpleSAMLphp and friends serve plain http.
	if u.Scheme != "https" && !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf("idp_sso_url must use https (plain http is allowed only for localhost)")
	}

	if strings.TrimSpace(c.IDPCertificate) == "" {
		return fmt.Errorf("idp_certificate is required when SAML is enabled")
	}
	if _, err := parseCertificate(c.IDPCertificate); err != nil {
		return fmt.Errorf("idp_certificate is not a valid PEM-encoded X.509 certificate: %w", err)
	}

	if c.DefaultRole == "" {
		return fmt.Errorf("default_role is required when SAML is enabled")
	}
	if err := validateDefaultRole(c.DefaultRole); err != nil {
		return err
	}

	return nil
}

// validateDefaultRole keeps just-in-time provisioning inside the roles a human
// user may hold, so a configuration change cannot mint a role the RBAC model
// does not recognise.
func validateDefaultRole(role string) error {
	for _, allowed := range types.UserTypeUser.AllowedRoles() {
		if string(allowed) == role {
			return nil
		}
	}
	return fmt.Errorf("default_role %q is not assignable to a user", role)
}

// validateStoredConfig checks a value arriving through the settings API, where
// it is an untyped map rather than a Config. Registered as the key's validator
// so a bad identity provider configuration is rejected at write time instead of
// at login, after the user has already authenticated.
func validateStoredConfig(value map[string]interface{}) error {
	cfg, err := configFromMap(value)
	if err != nil {
		return ierr.WithError(err).
			WithHint("saml_config is not a valid SAML configuration").
			Mark(ierr.ErrValidation)
	}
	if err := cfg.Validate(); err != nil {
		// Marked as a validation failure so a rejected configuration is
		// reported to the caller as a bad request rather than a server fault.
		return ierr.WithError(err).
			WithHint(err.Error()).
			Mark(ierr.ErrValidation)
	}
	return nil
}

// configFromMap decodes the stored JSONB blob into a Config.
func configFromMap(value map[string]interface{}) (Config, error) {
	cfg, err := utils.ToStruct[Config](value)
	if err != nil {
		return Config{}, fmt.Errorf("saml_config is not a valid SAML configuration: %w", err)
	}
	return cfg, nil
}

// isLoopbackHost reports whether the host never leaves this machine.
func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}
