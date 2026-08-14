package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/flexprice/flexprice/internal/types"
)

func TestSubModifyAddonParams_AddBinds(t *testing.T) {
	body := `{
		"type": "addon",
		"addon_params": {
			"action": "add",
			"add": {
				"addon_id": "addon_123",
				"cadence": "onetime",
				"proration_behavior": "create_prorations"
			}
		}
	}`

	var req ExecuteSubscriptionModifyRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid request, got: %v", err)
	}

	add := req.AddonParams.Add
	if add.AddonID != "addon_123" {
		t.Fatalf("addon_id did not bind, got %q", add.AddonID)
	}
	if add.Cadence != types.AddonCadenceOnetime {
		t.Fatalf("cadence did not bind, got %q", add.Cadence)
	}
	if add.ProrationBehavior != types.ProrationBehaviorCreateProrations {
		t.Fatalf("proration_behavior did not bind, got %q", add.ProrationBehavior)
	}
	if req.AddonParams.Remove != nil {
		t.Fatal("remove must stay nil on an add")
	}
}

func TestSubModifyAddonParams_RemoveBinds(t *testing.T) {
	body := `{
		"type": "addon",
		"addon_params": {
			"action": "remove",
			"remove": {
				"addon_association_id": "aa_123",
				"proration_behavior": "create_prorations",
				"reason": "downgrade"
			}
		}
	}`

	var req ExecuteSubscriptionModifyRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid request, got: %v", err)
	}

	remove := req.AddonParams.Remove
	if remove.AddonAssociationID != "aa_123" {
		t.Fatalf("addon_association_id did not bind, got %q", remove.AddonAssociationID)
	}
	if remove.Reason != "downgrade" {
		t.Fatalf("reason did not bind, got %q", remove.Reason)
	}
	if remove.ProrationBehavior != types.ProrationBehaviorCreateProrations {
		t.Fatalf("proration_behavior did not bind, got %q", remove.ProrationBehavior)
	}
	if req.AddonParams.Add != nil {
		t.Fatal("add must stay nil on a remove")
	}
}

func TestExecuteSubscriptionModifyRequest_AddonValidation(t *testing.T) {
	razorpay := &CheckoutParams{
		PaymentParams: PaymentParams{PaymentProvider: types.CheckoutPaymentProviderRazorpay},
	}

	tests := []struct {
		name    string
		req     ExecuteSubscriptionModifyRequest
		wantErr string
	}{
		{
			name:    "addon_params missing",
			req:     ExecuteSubscriptionModifyRequest{Type: SubscriptionModifyTypeAddon},
			wantErr: "addon_params is required",
		},
		{
			name: "unknown action",
			req: ExecuteSubscriptionModifyRequest{
				Type:        SubscriptionModifyTypeAddon,
				AddonParams: &SubModifyAddonParams{Action: "swap"},
			},
			wantErr: "unknown addon action",
		},
		{
			name: "add payload missing",
			req: ExecuteSubscriptionModifyRequest{
				Type:        SubscriptionModifyTypeAddon,
				AddonParams: &SubModifyAddonParams{Action: SubscriptionModificationActionAdd},
			},
			wantErr: "add is required",
		},
		{
			name: "remove payload missing",
			req: ExecuteSubscriptionModifyRequest{
				Type:        SubscriptionModifyTypeAddon,
				AddonParams: &SubModifyAddonParams{Action: SubscriptionModificationActionRemove},
			},
			wantErr: "remove is required",
		},
		{
			name: "add without addon_id",
			req: ExecuteSubscriptionModifyRequest{
				Type: SubscriptionModifyTypeAddon,
				AddonParams: &SubModifyAddonParams{
					Action: SubscriptionModificationActionAdd,
					Add:    &AddAddonToSubscriptionRequest{},
				},
			},
			wantErr: "AddonID",
		},
		{
			name: "remove without association id",
			req: ExecuteSubscriptionModifyRequest{
				Type: SubscriptionModifyTypeAddon,
				AddonParams: &SubModifyAddonParams{
					Action: SubscriptionModificationActionRemove,
					Remove: &RemoveAddonRequest{},
				},
			},
			wantErr: "AddonAssociationID",
		},
		{
			name: "remove rejects invalid proration behavior",
			req: ExecuteSubscriptionModifyRequest{
				Type: SubscriptionModifyTypeAddon,
				AddonParams: &SubModifyAddonParams{
					Action: SubscriptionModificationActionRemove,
					Remove: &RemoveAddonRequest{
						AddonAssociationID: "aa_1",
						ProrationBehavior:  "sometimes",
					},
				},
			},
			wantErr: "proration behavior",
		},
		{
			name: "checkout rejected on remove",
			req: ExecuteSubscriptionModifyRequest{
				Type: SubscriptionModifyTypeAddon,
				AddonParams: &SubModifyAddonParams{
					Action: SubscriptionModificationActionRemove,
					Remove: &RemoveAddonRequest{AddonAssociationID: "aa_1"},
				},
				Checkout: razorpay,
			},
			wantErr: "checkout is not supported when removing an addon",
		},
		{
			name: "checkout allowed on add",
			req: ExecuteSubscriptionModifyRequest{
				Type: SubscriptionModifyTypeAddon,
				AddonParams: &SubModifyAddonParams{
					Action: SubscriptionModificationActionAdd,
					Add:    &AddAddonToSubscriptionRequest{AddonID: "addon_1"},
				},
				Checkout: razorpay,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
