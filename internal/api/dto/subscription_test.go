package dto

import (
	"strings"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func TestLineItemCommitmentConfig_Validate_OverageFactor(t *testing.T) {
	amount := decimal.NewFromInt(100)

	t.Run("accepts overage factor of exactly 1.0", func(t *testing.T) {
		c := &LineItemCommitmentConfig{
			CommitmentAmount: &amount,
			CommitmentType:   types.COMMITMENT_TYPE_AMOUNT,
			OverageFactor:    lo.ToPtr(decimal.NewFromInt(1)),
		}
		if err := c.Validate(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("rejects overage factor below 1.0", func(t *testing.T) {
		c := &LineItemCommitmentConfig{
			CommitmentAmount: &amount,
			CommitmentType:   types.COMMITMENT_TYPE_AMOUNT,
			OverageFactor:    lo.ToPtr(decimal.NewFromFloat(0.5)),
		}
		err := c.Validate()
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "overage_factor must be at least 1.0") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func baseCreateSubscriptionRequest() CreateSubscriptionRequest {
	return CreateSubscriptionRequest{
		CustomerID:      "cust_test",
		PlanID:          "plan_test",
		Currency:        "usd",
		BillingPeriod:   types.BILLING_PERIOD_MONTHLY,
		BillingCycle:    types.BillingCycleAnniversary,
		StartDate:       nil,
		EndDate:         nil,
		BillingAnchor:   nil,
		PaymentBehavior: nil,
	}
}

func TestCreateSubscriptionRequestValidate_BillingAnchorRequiresAnniversaryBillingCycle(t *testing.T) {
	anchor := time.Now().UTC()

	t.Run("fails when billing_cycle is calendar", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		req.BillingCycle = types.BillingCycleCalendar
		req.BillingAnchor = &anchor

		err := req.Validate()
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}

		if !strings.Contains(err.Error(), "billing_anchor") {
			t.Fatalf("expected error to mention billing_anchor, got: %v", err)
		}
	})

	t.Run("passes when billing_cycle is anniversary", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		req.BillingCycle = types.BillingCycleAnniversary
		req.BillingAnchor = &anchor

		err := req.Validate()
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

func TestCreateSubscriptionRequestValidate_BillingAnchorOnOrAfterStartDate(t *testing.T) {
	start := time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC)

	t.Run("passes when billing_anchor equals start_date", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		req.StartDate = &start
		req.BillingCycle = types.BillingCycleAnniversary
		anchor := time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC)
		req.BillingAnchor = &anchor

		err := req.Validate()
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("passes when billing_anchor is after start_date", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		req.StartDate = &start
		req.BillingCycle = types.BillingCycleAnniversary
		anchor := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
		req.BillingAnchor = &anchor

		err := req.Validate()
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

func TestCancelSubscriptionRequest_Validate_BackdatedImmediate(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-5 * 24 * time.Hour)
	future := now.Add(5 * 24 * time.Hour)

	tests := []struct {
		name    string
		req     CancelSubscriptionRequest
		wantErr bool
	}{
		{
			name: "immediate_no_cancel_at_is_valid",
			req: CancelSubscriptionRequest{
				CancellationType:  types.CancellationTypeImmediate,
				ProrationBehavior: types.ProrationBehaviorNone,
			},
			wantErr: false,
		},
		{
			name: "immediate_past_cancel_at_is_valid",
			req: CancelSubscriptionRequest{
				CancellationType:  types.CancellationTypeImmediate,
				ProrationBehavior: types.ProrationBehaviorNone,
				CancelAt:          &past,
			},
			wantErr: false,
		},
		{
			name: "immediate_future_cancel_at_is_rejected",
			req: CancelSubscriptionRequest{
				CancellationType:  types.CancellationTypeImmediate,
				ProrationBehavior: types.ProrationBehaviorNone,
				CancelAt:          &future,
			},
			wantErr: true,
		},
		{
			name: "scheduled_date_past_cancel_at_is_valid",
			req: CancelSubscriptionRequest{
				CancellationType:  types.CancellationTypeScheduledDate,
				ProrationBehavior: types.ProrationBehaviorNone,
				CancelAt:          &past,
			},
			wantErr: false,
		},
		{
			name: "scheduled_date_future_cancel_at_is_valid",
			req: CancelSubscriptionRequest{
				CancellationType:  types.CancellationTypeScheduledDate,
				ProrationBehavior: types.ProrationBehaviorNone,
				CancelAt:          &future,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

func TestCreateSubscriptionRequestValidate_AutoInvoiceThreshold(t *testing.T) {
	t.Run("nil passes", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		if err := req.Validate(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("zero passes", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		z := decimal.Zero
		req.AutoInvoiceThreshold = &z
		if err := req.Validate(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("positive passes", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		p := decimal.RequireFromString("10")
		req.AutoInvoiceThreshold = &p
		if err := req.Validate(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("negative fails mentioning auto_invoice_threshold", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		n := decimal.NewFromInt(-1)
		req.AutoInvoiceThreshold = &n
		err := req.Validate()
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "auto_invoice_threshold") {
			t.Fatalf("expected error to mention auto_invoice_threshold, got: %v", err)
		}
	})
}

func TestSubscriptionInheritanceConfig_Validate_GroupedInvoicingChildrenToCreate(t *testing.T) {
	t.Run("rejects combining with subscriptions_ids_for_grouped_invoicing", func(t *testing.T) {
		c := &SubscriptionInheritanceConfig{
			GroupedInvoicingChildrenToCreate: []GroupedInvoicingChildRequest{
				{PlanID: "plan_seat", ExternalCustomerID: "ext_seat_1"},
			},
			SubscriptionsIDsForGroupedInvoicing: []string{"sub_existing_1"},
		}

		err := c.Validate()
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "grouped_invoicing_children_to_create") {
			t.Fatalf("expected error to mention grouped_invoicing_children_to_create, got: %v", err)
		}
	})

	t.Run("passes with only grouped_invoicing_children_to_create set", func(t *testing.T) {
		c := &SubscriptionInheritanceConfig{
			GroupedInvoicingChildrenToCreate: []GroupedInvoicingChildRequest{
				{PlanID: "plan_seat", ExternalCustomerID: "ext_seat_1"},
			},
		}

		err := c.Validate()
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("nil config still passes", func(t *testing.T) {
		var c *SubscriptionInheritanceConfig
		if err := c.Validate(); err != nil {
			t.Fatalf("expected no error for nil config, got: %v", err)
		}
	})
}

func TestCreateSubscriptionRequestValidate_GroupedInvoicingChildrenToCreate_RequiredFields(t *testing.T) {
	t.Run("rejects a child missing plan_id", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		req.Inheritance = &SubscriptionInheritanceConfig{
			GroupedInvoicingChildrenToCreate: []GroupedInvoicingChildRequest{
				{ExternalCustomerID: "ext_seat_1"},
			},
		}

		err := req.Validate()
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
	})

	t.Run("rejects a child missing external_customer_id", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		req.Inheritance = &SubscriptionInheritanceConfig{
			GroupedInvoicingChildrenToCreate: []GroupedInvoicingChildRequest{
				{PlanID: "plan_seat"},
			},
		}

		err := req.Validate()
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
	})

	t.Run("passes with both fields set", func(t *testing.T) {
		req := baseCreateSubscriptionRequest()
		req.Inheritance = &SubscriptionInheritanceConfig{
			GroupedInvoicingChildrenToCreate: []GroupedInvoicingChildRequest{
				{PlanID: "plan_seat", ExternalCustomerID: "ext_seat_1"},
			},
		}

		if err := req.Validate(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

func razorpayCheckoutParams() *CheckoutParams {
	return &CheckoutParams{
		PaymentParams: PaymentParams{
			PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		},
		RedirectionParams: RedirectionParams{
			SuccessURL: lo.ToPtr("https://app.example.com/ok"),
		},
	}
}

func TestCreateSubscriptionRequestValidate_CheckoutAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CreateSubscriptionRequest)
		wantErr string
	}{
		{
			name:    "plain checkout accepted",
			mutate:  func(r *CreateSubscriptionRequest) {},
			wantErr: "",
		},
		{
			name:    "subscription_status rejected",
			mutate:  func(r *CreateSubscriptionRequest) { r.SubscriptionStatus = types.SubscriptionStatusDraft },
			wantErr: "subscription_status",
		},
		{
			name: "phases rejected",
			mutate: func(r *CreateSubscriptionRequest) {
				r.Phases = []SubscriptionPhaseCreateRequest{{StartDate: time.Now().UTC()}}
			},
			wantErr: "phases",
		},
		{
			// Lifetime config read at every renewal, not an instruction for this payment.
			name: "payment_behavior accepted",
			mutate: func(r *CreateSubscriptionRequest) {
				r.PaymentBehavior = lo.ToPtr(types.PaymentBehaviorDefaultActive)
			},
			wantErr: "",
		},
		{
			// The saved method for renewals; the session collects the opening charge itself.
			name:    "gateway_payment_method_id accepted",
			mutate:  func(r *CreateSubscriptionRequest) { r.GatewayPaymentMethodID = lo.ToPtr("pm_123") },
			wantErr: "",
		},
		{
			// Governs future invoices, a different question from how this checkout collects.
			name: "collection_method accepted",
			mutate: func(r *CreateSubscriptionRequest) {
				r.CollectionMethod = lo.ToPtr(types.CollectionMethodSendInvoice)
			},
			wantErr: "",
		},
		{
			// A trial produces no charge, so the service falls through to a normal create rather
			// than rejecting.
			name:    "trial_period_days accepted",
			mutate:  func(r *CreateSubscriptionRequest) { r.TrialPeriodDays = lo.ToPtr(14) },
			wantErr: "",
		},
		{
			name: "inheritance rejected",
			mutate: func(r *CreateSubscriptionRequest) {
				r.Inheritance = &SubscriptionInheritanceConfig{
					GroupedInvoicingChildrenToCreate: []GroupedInvoicingChildRequest{
						{PlanID: "plan_seat", ExternalCustomerID: "ext_seat_1"},
					},
				}
			},
			wantErr: "inheritance",
		},
		{
			name:    "missing payment_provider rejected",
			mutate:  func(r *CreateSubscriptionRequest) { r.Checkout.PaymentProvider = "" },
			wantErr: "PaymentProvider",
		},
		{
			// Struct tags only enforce presence; the enum allow-list lives in
			// PaymentParams.Validate, which is why the allowlist calls Checkout.Validate itself.
			name:    "unsupported payment_provider rejected",
			mutate:  func(r *CreateSubscriptionRequest) { r.Checkout.PaymentProvider = "stripe" },
			wantErr: "payment provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := baseCreateSubscriptionRequest()
			req.Checkout = razorpayCheckoutParams()
			tt.mutate(&req)

			err := req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to mention %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// Everything the allowlist rejects must stay legal without a checkout object.
func TestCreateSubscriptionRequestValidate_NoCheckoutUnaffected(t *testing.T) {
	req := baseCreateSubscriptionRequest()
	req.SubscriptionStatus = types.SubscriptionStatusDraft
	req.Inheritance = &SubscriptionInheritanceConfig{
		GroupedInvoicingChildrenToCreate: []GroupedInvoicingChildRequest{
			{PlanID: "plan_seat", ExternalCustomerID: "ext_seat_1"},
		},
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("expected no error without checkout, got: %v", err)
	}
}

// The allowlist runs before Validate() defaults collection_method and payment_behavior. If it ran
// after, a caller who sent neither would be judged against the values Validate() filled in.
func TestCreateSubscriptionRequestValidate_CheckoutRunsBeforeDefaulting(t *testing.T) {
	req := baseCreateSubscriptionRequest()
	req.Checkout = razorpayCheckoutParams()
	req.CollectionMethod = nil
	req.PaymentBehavior = nil

	if err := req.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if req.CollectionMethod == nil || req.PaymentBehavior == nil {
		t.Fatal("expected Validate to still apply its defaults on the checkout path")
	}
}
