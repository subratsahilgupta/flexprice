package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/addonassociation"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func TestAddAddonToSubscriptionRequest_Validate_RejectsDuplicateOverridePriceIDs(t *testing.T) {
	req := &AddAddonToSubscriptionRequest{
		AddonID: "addon_123",
		OverrideLineItems: []OverrideLineItemRequest{
			{PriceID: "price_1", Amount: lo.ToPtr(decimal.NewFromInt(10))},
			{PriceID: "price_1", Amount: lo.ToPtr(decimal.NewFromInt(20))},
		},
	}

	err := req.Validate()
	if err == nil {
		t.Fatal("expected validation error for duplicate override price_id, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate price_id in override line items") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddAddonToSubscriptionRequest_Validate_AcceptsDistinctOverridePriceIDs(t *testing.T) {
	req := &AddAddonToSubscriptionRequest{
		AddonID: "addon_123",
		OverrideLineItems: []OverrideLineItemRequest{
			{PriceID: "price_1", Amount: lo.ToPtr(decimal.NewFromInt(10))},
			{PriceID: "price_2", Amount: lo.ToPtr(decimal.NewFromInt(20))},
		},
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// The association is embedded as a POINTER so that encoding/json omits it entirely when nil.
// If it were embedded by value, every pay-later response would grow a wall of zero-valued
// fields — a wire-format change for existing clients of POST /subscriptions/addon.
func TestAddAddonToSubscriptionResponse_MarshalsFlatAndOmitsNils(t *testing.T) {
	t.Run("with association, flat legacy shape", func(t *testing.T) {
		start := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
		resp := &AddAddonToSubscriptionResponse{
			AddonAssociation: &addonassociation.AddonAssociation{
				ID:          "addon_assoc_1",
				EntityID:    "subs_1",
				EntityType:  types.AddonAssociationEntityTypeSubscription,
				AddonID:     "addon_1",
				AddonStatus: types.AddonStatusActive,
				StartDate:   &start,
			},
		}

		out, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var decoded map[string]any
		if err := json.Unmarshal(out, &decoded); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// Association fields sit at the top level, not under a nested key.
		if decoded["id"] != "addon_assoc_1" {
			t.Fatalf("expected flat id at top level, got: %s", out)
		}
		if decoded["addon_status"] != string(types.AddonStatusActive) {
			t.Fatalf("expected flat addon_status, got: %s", out)
		}
		if _, ok := decoded["checkout_session"]; ok {
			t.Fatalf("checkout_session must be omitted when nil, got: %s", out)
		}
		if _, ok := decoded["invoice"]; ok {
			t.Fatalf("invoice must be omitted when nil, got: %s", out)
		}
	})

	t.Run("nil association marshals to an empty object", func(t *testing.T) {
		out, err := json.Marshal(&AddAddonToSubscriptionResponse{})
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		if string(out) != "{}" {
			t.Fatalf("expected {} for a nil embedded association, got: %s", out)
		}
	})

	t.Run("pay-first carries checkout session and invoice", func(t *testing.T) {
		resp := &AddAddonToSubscriptionResponse{
			AddonAssociation: &addonassociation.AddonAssociation{
				ID:          "addon_assoc_2",
				AddonStatus: types.AddonStatusPending,
			},
			CheckoutSession: &CheckoutSessionResponse{},
			Invoice:         &InvoiceResponse{},
		}

		out, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var decoded map[string]any
		if err := json.Unmarshal(out, &decoded); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if _, ok := decoded["checkout_session"]; !ok {
			t.Fatalf("expected checkout_session in the pay-first body, got: %s", out)
		}
		if _, ok := decoded["invoice"]; !ok {
			t.Fatalf("expected invoice in the pay-first body, got: %s", out)
		}
		if decoded["addon_status"] != string(types.AddonStatusPending) {
			t.Fatalf("expected pending addon_status, got: %s", out)
		}
	})
}
