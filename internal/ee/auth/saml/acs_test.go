package saml

import (
	"os"
	"strings"
	"testing"

	"github.com/crewjam/saml"
)

func assertionWithNameID(value string) *saml.Assertion {
	return &saml.Assertion{
		Subject: &saml.Subject{
			NameID: &saml.NameID{Value: value},
		},
	}
}

func assertionWithAttribute(name, value string) *saml.Assertion {
	return &saml.Assertion{
		AttributeStatements: []saml.AttributeStatement{
			{
				Attributes: []saml.Attribute{
					{
						Name:   name,
						Values: []saml.AttributeValue{{Value: value}},
					},
				},
			},
		},
	}
}

// TestExtractEmailFromNameID covers the default path, where the identity
// provider sends the email as the NameID.
func TestExtractEmailFromNameID(t *testing.T) {
	got, err := extractEmail(assertionWithNameID("Alice@Example.com"), Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Casing is normalised so an identity provider that changes case cannot
	// create a second account for someone who already exists.
	if got != "alice@example.com" {
		t.Errorf("got %q, want the lowercased address", got)
	}
}

func TestExtractEmailFromConfiguredAttribute(t *testing.T) {
	cfg := Config{EmailAttribute: "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress"}
	assertion := assertionWithAttribute(cfg.EmailAttribute, "bob@example.com")

	got, err := extractEmail(assertion, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "bob@example.com" {
		t.Errorf("got %q, want bob@example.com", got)
	}
}

// TestExtractEmailRejectsUnusableAssertions is the important one: an assertion
// that authenticates someone we cannot identify must fail, never fall back to a
// guess. A wrong answer here logs a user into the wrong account.
func TestExtractEmailRejectsUnusableAssertions(t *testing.T) {
	cases := []struct {
		name      string
		assertion *saml.Assertion
		cfg       Config
	}{
		{
			name:      "no subject at all",
			assertion: &saml.Assertion{},
		},
		{
			name:      "NameID is an opaque identifier, not an email",
			assertion: assertionWithNameID("a7f3c9e1-persistent-id"),
		},
		{
			name:      "NameID empty",
			assertion: assertionWithNameID("   "),
		},
		{
			name:      "configured attribute absent",
			assertion: assertionWithAttribute("some_other_claim", "carol@example.com"),
			cfg:       Config{EmailAttribute: "email"},
		},
		{
			name:      "configured attribute present but empty",
			assertion: assertionWithAttribute("email", "  "),
			cfg:       Config{EmailAttribute: "email"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractEmail(tc.assertion, tc.cfg)
			if err == nil {
				t.Fatalf("expected rejection, got email %q", got)
			}
		})
	}
}

// TestConfiguredAttributeDoesNotFallBackToNameID pins a deliberate choice: when
// an administrator names an email attribute, a NameID must not silently satisfy
// it. Falling back would mean a misconfigured attribute quietly authenticates
// users off whatever the identity provider happens to put in the NameID.
func TestConfiguredAttributeDoesNotFallBackToNameID(t *testing.T) {
	assertion := assertionWithNameID("dave@example.com")
	cfg := Config{EmailAttribute: "email"}

	if got, err := extractEmail(assertion, cfg); err == nil {
		t.Errorf("configured attribute fell back to NameID, yielding %q", got)
	}
}

// TestResolveUserLooksUpAcrossTenants pins the fix for a real failure found by
// live testing: the lookup ran with the assertion's tenant in context, and
// GetByEmail filters by tenant when one is set. A user belonging to another
// organisation was therefore invisible, so the flow fell through to
// provisioning and hit the global unique index on users.email — surfacing as a
// 500 database error instead of the explicit 403 refusal.
//
// The lookup must clear the tenant so the cross-tenant branch is reachable.
func TestResolveUserLooksUpAcrossTenants(t *testing.T) {
	src, err := os.ReadFile("acs.go")
	if err != nil {
		t.Fatalf("read acs.go: %v", err)
	}

	if !strings.Contains(string(src), `GetByEmail(types.SetTenantID(ctx, "")`) {
		t.Error("resolveUser must look the user up with no tenant in context, " +
			"otherwise a user in another organisation is invisible and the " +
			"cross-tenant refusal never fires")
	}
}

// TestExtractEmailRejectsNonEmailAttribute closes the gap between the two
// extraction paths. The NameID path has always required an "@"; the attribute
// path did not, so an identity provider mapping the configured attribute to a
// username or an employee number produced a user whose email column held that
// value. users.email is globally unique, so the row is permanent and blocks the
// real address from ever being provisioned.
func TestExtractEmailRejectsNonEmailAttribute(t *testing.T) {
	cfg := Config{EmailAttribute: "email"}

	for _, value := range []string{"jsmith", "EMP-00417", "user1"} {
		if _, err := extractEmail(assertionWithAttribute("email", value), cfg); err == nil {
			t.Errorf("attribute value %q was accepted as an email address", value)
		}
	}

	// A real address still works, and is still normalised.
	got, err := extractEmail(assertionWithAttribute("email", "  Alice@Example.com "), cfg)
	if err != nil {
		t.Fatalf("a valid address must be accepted: %v", err)
	}
	if got != "alice@example.com" {
		t.Errorf("extractEmail() = %q, want it trimmed and lowercased", got)
	}
}
