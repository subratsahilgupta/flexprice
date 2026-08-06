package service

import (
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	domainCheckout "github.com/flexprice/flexprice/internal/domain/checkout"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// seedFixedPricePlan registers a plan with one FIXED ADVANCE price — the shape that actually
// produces a charge on the opening invoice, unlike the suite's default usage-only plan.
//
// trialDays > 0 puts the trial on the PRICE rather than the request, which is the case the DTO
// allowlist cannot see and the create path resolves later.
func (s *SubscriptionServiceSuite) seedFixedPricePlan(
	planID string,
	amount decimal.Decimal,
	trialDays int,
) *plan.Plan {
	ctx := s.GetContext()

	p := &plan.Plan{
		ID:        planID,
		Name:      "Fixed Price Plan",
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PlanRepo.Create(ctx, p))

	s.NoError(s.GetStores().PriceRepo.Create(ctx, &price.Price{
		ID:                 "price_" + planID,
		Amount:             amount,
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           planID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		TrialPeriodDays:    trialDays,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}))

	return p
}

// seedUsageOnlyPlan registers a plan whose only price is metered and billed in arrears, so its
// opening invoice has no charges at all and ComputeInvoice reports it skipped. The suite's default
// plan cannot stand in for this — it carries price_fixed_monthly.
func (s *SubscriptionServiceSuite) seedUsageOnlyPlan(planID string) *plan.Plan {
	ctx := s.GetContext()

	p := &plan.Plan{
		ID:        planID,
		Name:      "Usage Only Plan",
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PlanRepo.Create(ctx, p))

	s.NoError(s.GetStores().PriceRepo.Create(ctx, &price.Price{
		ID:                 "price_" + planID,
		Amount:             decimal.NewFromFloat(0.1),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           planID,
		Type:               types.PRICE_TYPE_USAGE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		InvoiceCadence:     types.InvoiceCadenceArrear,
		MeterID:            s.testData.meters.apiCalls.ID,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}))

	return p
}

func (s *SubscriptionServiceSuite) checkoutCreateRequest(planID string) dto.CreateSubscriptionRequest {
	return dto.CreateSubscriptionRequest{
		CustomerID:    s.testData.customer.ID,
		PlanID:        planID,
		Currency:      "usd",
		BillingPeriod: types.BILLING_PERIOD_MONTHLY,
		BillingCycle:  types.BillingCycleAnniversary,
		Checkout:      s.razorpayCheckoutParams(),
	}
}

// invoicesForSubscription lists every invoice regardless of status. The statuses are spelled out
// because an unfiltered list silently drops SKIPPED invoices — which is exactly what a zero-charge
// gated create produces.
func (s *SubscriptionServiceSuite) invoicesForSubscription(subscriptionID string) []*invoice.Invoice {
	filter := types.NewNoLimitInvoiceFilter()
	filter.SubscriptionID = subscriptionID
	filter.InvoiceStatus = []types.InvoiceStatus{
		types.InvoiceStatusDraft,
		types.InvoiceStatusFinalized,
		types.InvoiceStatusVoided,
		types.InvoiceStatusSkipped,
	}
	invoices, err := s.GetStores().InvoiceRepo.List(s.GetContext(), filter)
	s.NoError(err)
	return invoices
}

// ─────────────────────────────────────────────
// Pay-later must be untouched
// ─────────────────────────────────────────────

func (s *SubscriptionServiceSuite) TestCreateSubscription_WithoutCheckoutIsUnchanged() {
	ctx := s.GetContext()
	s.seedFixedPricePlan("plan_paylater_create", decimal.NewFromInt(50), 0)

	req := s.checkoutCreateRequest("plan_paylater_create")
	req.Checkout = nil

	resp, err := s.service.CreateSubscription(ctx, req)
	s.Require().NoError(err)

	s.NotEqual(types.SubscriptionStatusDraft, resp.SubscriptionStatus,
		"a create without checkout must not be gated behind payment")
	s.Nil(resp.CheckoutSession, "no session may be opened when checkout is omitted")

	sessions, err := s.GetStores().CheckoutSessionRepo.List(ctx, &types.CheckoutSessionFilter{
		QueryFilter: types.NewNoLimitPublishedQueryFilter(),
	})
	s.Require().NoError(err)
	s.Empty(sessions)
}

// ─────────────────────────────────────────────
// Nothing to collect → activate immediately
// ─────────────────────────────────────────────

// A usage-only plan bills in arrears, so the opening invoice has no charges at all and
// ComputeInvoice reports it skipped.
func (s *SubscriptionServiceSuite) TestCreateSubscriptionWithCheckout_UsageOnlyPlanActivatesImmediately() {
	ctx := s.GetContext()
	s.seedUsageOnlyPlan("plan_usage_only_checkout")

	resp, err := s.service.CreateSubscription(ctx, s.checkoutCreateRequest("plan_usage_only_checkout"))
	s.Require().NoError(err)

	s.Equal(types.SubscriptionStatusActive, resp.SubscriptionStatus,
		"nothing to collect means the subscription activates without a payment gate")
	s.Nil(resp.CheckoutSession, "no session may be opened when there is no charge")

	sessions, err := s.GetStores().CheckoutSessionRepo.List(ctx, &types.CheckoutSessionFilter{
		QueryFilter: types.NewNoLimitPublishedQueryFilter(),
	})
	s.Require().NoError(err)
	s.Empty(sessions, "a zero-charge create must create no checkout session at all")

	invoices := s.invoicesForSubscription(resp.ID)
	s.Require().NotEmpty(invoices, "the checkout path prices a draft before deciding not to gate")
	for _, inv := range invoices {
		s.Equal(types.StatusDeleted, inv.Status,
			"the empty draft priced for the checkout must not be left behind")
	}
}

// ─────────────────────────────────────────────
// Trials fall through instead of being gated
// ─────────────────────────────────────────────

func (s *SubscriptionServiceSuite) TestCreateSubscriptionWithCheckout_ExplicitTrialFallsThrough() {
	ctx := s.GetContext()
	s.seedFixedPricePlan("plan_trial_explicit", decimal.NewFromInt(50), 0)

	req := s.checkoutCreateRequest("plan_trial_explicit")
	req.TrialPeriodDays = lo.ToPtr(14)

	resp, err := s.service.CreateSubscription(ctx, req)
	s.Require().NoError(err)

	s.Equal(types.SubscriptionStatusTrialing, resp.SubscriptionStatus,
		"a trial raises no charge, so the create must not be gated")
	s.Nil(resp.CheckoutSession)
	s.NotNil(resp.TrialEnd, "the trial window must survive the checkout path")
}

// The regression that motivates resolving the trial before forcing draft:
// syncTrialingStateFromCreateRequest returns early for draft, so a plan-inherited trial would be
// flattened into a plain draft and later activated as ACTIVE — silently losing the trial.
func (s *SubscriptionServiceSuite) TestCreateSubscriptionWithCheckout_PlanInheritedTrialFallsThrough() {
	ctx := s.GetContext()
	s.seedFixedPricePlan("plan_trial_inherited", decimal.NewFromInt(50), 30)

	// Note: no trial_period_days on the request — it comes from the plan's price.
	resp, err := s.service.CreateSubscription(ctx, s.checkoutCreateRequest("plan_trial_inherited"))
	s.Require().NoError(err)

	s.Equal(types.SubscriptionStatusTrialing, resp.SubscriptionStatus,
		"a trial inherited from the plan's prices must not be flattened into a draft")
	s.Nil(resp.CheckoutSession)
	s.Require().NotNil(resp.TrialEnd)
	s.Equal(30, int(resp.TrialEnd.Sub(lo.FromPtr(resp.TrialStart)).Hours()/24))
}

// ─────────────────────────────────────────────
// Pay-first rollback
// ─────────────────────────────────────────────

// seedCollidingCheckoutSession occupies the partial-unique idempotency key so the next session
// Create for it fails. This is the only way to drive StartPayFirstCheckoutSession's failure paths
// without a live Razorpay connection — integration.Factory is a concrete struct with no seam for a
// fake checkout provider.
func (s *SubscriptionServiceSuite) seedCollidingCheckoutSession(idempKey string) {
	ctx := s.GetContext()
	s.Require().NoError(s.GetStores().CheckoutSessionRepo.Create(ctx, &domainCheckout.CheckoutSession{
		ID:              types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CHECKOUT_SESSION),
		EnvironmentID:   types.GetEnvironmentID(ctx),
		CustomerID:      s.testData.customer.ID,
		Action:          types.CheckoutActionCreateSubscription,
		CheckoutStatus:  types.CheckoutStatusPending,
		PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		Configuration: domainCheckout.ToJSONBCheckoutConfiguration(types.CheckoutConfiguration{
			CreateSubscriptionParams: &types.CreateSubscriptionParams{
				SubscriptionID: "subs_some_other_subscription",
			},
		}),
		IdempotencyKey: &idempKey,
		ExpiresAt:      time.Now().UTC().Add(time.Hour),
		BaseModel:      types.GetDefaultBaseModel(ctx),
	}))
}

// subscriptionForPlan finds the subscription a gated create wrote, including archived ones — the
// rollback path soft deletes, so the row is still there.
func (s *SubscriptionServiceSuite) subscriptionForPlan(planID string) *subscription.Subscription {
	subs, err := s.GetStores().SubscriptionRepo.List(s.GetContext(), types.NewNoLimitSubscriptionFilter())
	s.Require().NoError(err)

	for _, sub := range subs {
		if sub.PlanID == planID {
			return sub
		}
	}
	return nil
}

// draftInvoiceForPlan finds the invoice the gated create priced, by the plan its subscription used.
func (s *SubscriptionServiceSuite) draftInvoiceForPlan(planID string) *invoice.Invoice {
	sub := s.subscriptionForPlan(planID)
	if sub == nil {
		return nil
	}

	invoices := s.invoicesForSubscription(sub.ID)
	if len(invoices) == 0 {
		return nil
	}
	return invoices[0]
}

// The whole point of gating on POST /subscriptions rather than POST /checkout/sessions: the amount
// the customer is asked to pay includes the addons the slim legacy params could not express.
//
// Asserted on the archived draft, because the run cannot get past the provider call.
func (s *SubscriptionServiceSuite) TestCreateSubscriptionWithCheckout_AddonIsPricedIntoTheDraft() {
	ctx := s.GetContext()
	s.seedFixedPricePlan("plan_addon_priced", decimal.NewFromInt(50), 0)
	s.seedFixedPriceAddon("addon_priced_into_draft", decimal.NewFromInt(30), types.InvoiceCadenceAdvance)

	idempKey := "create-subscription-addon-priced-idemp-key"
	s.seedCollidingCheckoutSession(idempKey)

	req := s.checkoutCreateRequest("plan_addon_priced")
	req.Checkout.IdempotencyKey = &idempKey
	req.Addons = []dto.AddAddonToSubscriptionRequest{{
		AddonID: "addon_priced_into_draft",
		Cadence: types.AddonCadenceRecurring,
	}}

	_, err := s.service.CreateSubscription(ctx, req)
	s.Require().Error(err, "expected the seeded idempotency collision to fail session create")

	inv := s.draftInvoiceForPlan("plan_addon_priced")
	s.Require().NotNil(inv, "the gated create must have priced a draft invoice before opening a session")
	s.True(inv.AmountDue.Equal(decimal.NewFromInt(80)),
		"the locked amount must cover plan (50) + addon (30), got %s", inv.AmountDue)
}

// Drives StartPayFirstCheckoutSession's session-create failure by colliding on the partial-unique
// idempotency key, which is reachable without a live Razorpay connection.
func (s *SubscriptionServiceSuite) TestCreateSubscriptionWithCheckout_SessionCreateFailureArchivesDraft() {
	ctx := s.GetContext()
	s.seedFixedPricePlan("plan_payfirst_rollback", decimal.NewFromInt(50), 0)

	idempKey := "create-subscription-orphan-idemp-key"
	s.seedCollidingCheckoutSession(idempKey)

	req := s.checkoutCreateRequest("plan_payfirst_rollback")
	req.Checkout.IdempotencyKey = &idempKey

	resp, err := s.service.CreateSubscription(ctx, req)
	s.Require().Error(err)
	s.True(ierr.IsAlreadyExists(err), "expected the seeded idempotency collision, got %v", err)
	s.Nil(resp)

	// Asserted on status rather than absence: both repositories soft delete, so the rows stay
	// readable and "is it gone" would pass whether or not the rollback ran.
	sub := s.subscriptionForPlan("plan_payfirst_rollback")
	s.Require().NotNil(sub, "the subscription is written before the session, so it must exist")
	s.Equal(types.StatusArchived, sub.Status,
		"the draft subscription must be archived when pay-first setup fails")
	s.Equal(types.SubscriptionStatusDraft, sub.SubscriptionStatus,
		"the soft delete leaves subscription_status untouched, matching the ent repository")

	invoices := s.invoicesForSubscription(sub.ID)
	s.Require().NotEmpty(invoices, "the draft invoice is priced before the session is opened")
	for _, inv := range invoices {
		s.Equal(types.StatusDeleted, inv.Status,
			"the draft invoice must be archived alongside its subscription")
	}
}
