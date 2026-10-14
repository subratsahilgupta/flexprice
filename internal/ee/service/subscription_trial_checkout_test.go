package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	domainCheckout "github.com/flexprice/flexprice/internal/domain/checkout"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrialConversionPeriod(t *testing.T) {
	start := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		cycle      types.BillingCycle
		wantAnchor time.Time
		wantEnd    time.Time
	}{
		{"calendar", types.BillingCycleCalendar, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"anniversary", types.BillingCycleAnniversary, start, time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &subscription.Subscription{BillingCycle: tt.cycle, BillingPeriod: types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1, Timezone: "UTC"}
			anchor, end, err := trialConversionPeriod(sub, start)
			require.NoError(t, err)
			assert.True(t, anchor.Equal(tt.wantAnchor), "anchor %s", anchor)
			assert.True(t, end.Equal(tt.wantEnd), "end %s", end)
			assert.True(t, sub.BillingAnchor.IsZero(), "helper must not mutate the subscription")
		})
	}
}

// seedTrialingFixedFeeSubscription creates a trialing monthly subscription (13 days left) with one fixed advance fee,
// and points checkout at a fake gateway so pay-first sessions can start.
func (s *SubscriptionServiceSuite) seedTrialingFixedFeeSubscription(amount decimal.Decimal) *subscription.Subscription {
	ctx := s.GetContext()
	s.GetIntegrationFactory().SetCheckoutProvider(&fakeCheckoutProvider{})
	now := time.Now().UTC()
	trialStart, trialEnd := now.Add(-24*time.Hour), now.Add(13*24*time.Hour)

	pr := &price.Price{
		ID: types.GenerateUUIDWithPrefix(types.UUID_PREFIX_PRICE), Amount: amount, Currency: "usd",
		EntityType: types.PRICE_ENTITY_TYPE_PLAN, EntityID: s.testData.plan.ID, Type: types.PRICE_TYPE_FIXED,
		BillingPeriod: types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1, BillingModel: types.BILLING_MODEL_FLAT_FEE,
		InvoiceCadence: types.InvoiceCadenceAdvance, BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.Require().NoError(s.GetStores().PriceRepo.Create(ctx, pr))

	sub := &subscription.Subscription{
		ID: types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION), PlanID: s.testData.plan.ID,
		CustomerID: s.testData.customer.ID, StartDate: trialStart, BillingAnchor: trialStart,
		CurrentPeriodStart: trialStart, CurrentPeriodEnd: trialEnd, TrialStart: &trialStart, TrialEnd: &trialEnd,
		Currency: "usd", BillingCycle: types.BillingCycleAnniversary, BillingPeriod: types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1, BillingCadence: types.BILLING_CADENCE_RECURRING,
		SubscriptionStatus: types.SubscriptionStatusTrialing,
		PaymentBehavior:    string(types.PaymentBehaviorDefaultActive), CollectionMethod: string(types.CollectionMethodChargeAutomatically),
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	li := &subscription.SubscriptionLineItem{
		ID: types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM), SubscriptionID: sub.ID,
		CustomerID: sub.CustomerID, EntityID: s.testData.plan.ID, EntityType: types.SubscriptionLineItemEntityTypePlan,
		PlanDisplayName: s.testData.plan.Name, PriceID: pr.ID, PriceType: pr.Type, DisplayName: "Platform fee",
		Quantity: decimal.NewFromInt(1), Currency: "usd", BillingPeriod: types.BILLING_PERIOD_MONTHLY,
		InvoiceCadence: types.InvoiceCadenceAdvance, StartDate: trialStart, BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.Require().NoError(s.GetStores().SubscriptionRepo.CreateWithLineItems(ctx, sub, []*subscription.SubscriptionLineItem{li}))
	return sub
}

func (s *SubscriptionServiceSuite) trialEndCheckoutRequest() dto.ExecuteSubscriptionModifyRequest {
	return dto.ExecuteSubscriptionModifyRequest{
		Type:           dto.SubscriptionModifyTypeTrialEnd,
		TrialEndParams: &dto.SubModifyTrialEndRequest{Action: dto.TrialEndActionImmediate},
		Checkout:       s.razorpayCheckoutParams(),
	}
}

func (s *SubscriptionServiceSuite) TestTrialEndCheckout_GatesWithoutTouchingTrial() {
	ctx := s.GetContext()
	sub := s.seedTrialingFixedFeeSubscription(decimal.NewFromInt(200))
	modSvc := NewSubscriptionModificationService(s.service.(*subscriptionService).ServiceParams)

	resp, err := modSvc.Execute(ctx, sub.ID, s.trialEndCheckoutRequest())
	s.Require().NoError(err)
	s.Require().NotNil(resp.CheckoutSession, "a positive first-period invoice is gated")

	got, err := s.GetStores().SubscriptionRepo.Get(ctx, sub.ID)
	s.Require().NoError(err)
	s.Equal(types.SubscriptionStatusTrialing, got.SubscriptionStatus, "trial untouched until payment")
	s.True(got.TrialEnd.Equal(*sub.TrialEnd))

	draft, err := s.GetStores().InvoiceRepo.Get(ctx, lo.FromPtr(resp.CheckoutSession.CheckoutInvoiceID))
	s.Require().NoError(err)
	s.Equal(types.InvoiceStatusDraft, draft.InvoiceStatus)
	s.Equal(string(types.InvoiceBillingReasonSubscriptionTrialEnd), draft.BillingReason)
	s.True(draft.Total.Equal(decimal.NewFromInt(200)), "full first-period fee, got %s", draft.Total)
}

func (s *SubscriptionServiceSuite) TestTrialEndCheckout_SecondStartRejected() {
	ctx := s.GetContext()
	sub := s.seedTrialingFixedFeeSubscription(decimal.NewFromInt(200))
	modSvc := NewSubscriptionModificationService(s.service.(*subscriptionService).ServiceParams)
	_, err := modSvc.Execute(ctx, sub.ID, s.trialEndCheckoutRequest())
	s.Require().NoError(err)
	_, err = modSvc.Execute(ctx, sub.ID, s.trialEndCheckoutRequest())
	s.True(ierr.IsAlreadyExists(err), "one open checkout per subscription")
}

func (s *SubscriptionServiceSuite) TestTrialEndCheckout_DirectEndWhilePendingRejected() {
	ctx := s.GetContext()
	sub := s.seedTrialingFixedFeeSubscription(decimal.NewFromInt(200))
	modSvc := NewSubscriptionModificationService(s.service.(*subscriptionService).ServiceParams)
	_, err := modSvc.Execute(ctx, sub.ID, s.trialEndCheckoutRequest())
	s.Require().NoError(err)

	req := s.trialEndCheckoutRequest()
	req.Checkout = nil
	_, err = modSvc.Execute(ctx, sub.ID, req)
	s.True(ierr.IsAlreadyExists(err), "the pending checkout owns the trial end")
	got, err := s.GetStores().SubscriptionRepo.Get(ctx, sub.ID)
	s.Require().NoError(err)
	s.Equal(types.SubscriptionStatusTrialing, got.SubscriptionStatus)
}

func (s *SubscriptionServiceSuite) TestTrialEndCheckout_AlreadyEndedTrialRejected() {
	ctx := s.GetContext()
	sub := s.seedTrialingFixedFeeSubscription(decimal.NewFromInt(200))
	sub.SubscriptionStatus = types.SubscriptionStatusIncomplete
	s.Require().NoError(s.GetStores().SubscriptionRepo.Update(ctx, sub))
	modSvc := NewSubscriptionModificationService(s.service.(*subscriptionService).ServiceParams)
	_, err := modSvc.Execute(ctx, sub.ID, s.trialEndCheckoutRequest())
	s.True(ierr.IsValidation(err))
}

func (s *SubscriptionServiceSuite) TestTrialEndCheckout_ScheduledDateWithCheckoutRejected() {
	req := s.trialEndCheckoutRequest()
	future := time.Now().UTC().Add(48 * time.Hour)
	req.TrialEndParams = &dto.SubModifyTrialEndRequest{Action: dto.TrialEndActionScheduledDate, NewTrialEnd: &future}
	s.True(ierr.IsValidation(req.Validate()))
}

func (s *SubscriptionServiceSuite) TestTrialEndCheckout_ZeroInvoiceEndsTrialImmediately() {
	ctx := s.GetContext()
	sub := s.seedTrialingFixedFeeSubscription(decimal.Zero)
	modSvc := NewSubscriptionModificationService(s.service.(*subscriptionService).ServiceParams)
	resp, err := modSvc.Execute(ctx, sub.ID, s.trialEndCheckoutRequest())
	s.Require().NoError(err)
	s.Nil(resp.CheckoutSession, "nothing to collect")
	got, _ := s.GetStores().SubscriptionRepo.Get(ctx, sub.ID)
	s.NotEqual(types.SubscriptionStatusTrialing, got.SubscriptionStatus)
}

// seedPendingModifyCheckout stores an open modify_subscription checkout of the given type for the subscription.
func (s *SubscriptionServiceSuite) seedPendingModifyCheckout(sub *subscription.Subscription, modifyType types.ModifySubscriptionType) {
	ctx := s.GetContext()
	s.Require().NoError(s.GetStores().CheckoutSessionRepo.Create(ctx, &domainCheckout.CheckoutSession{
		ID:              types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CHECKOUT_SESSION),
		CustomerID:      sub.CustomerID,
		Action:          types.CheckoutActionModifySubscription,
		CheckoutStatus:  types.CheckoutStatusPending,
		PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		Configuration: domainCheckout.JSONBCheckoutConfiguration{
			ModifySubscriptionParams: &types.ModifySubscriptionParams{SubscriptionID: sub.ID, ModifyType: modifyType},
		},
		ExpiresAt:     time.Now().UTC().Add(20 * time.Minute),
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}))
}

func (s *SubscriptionServiceSuite) TestTrialEndCheckout_CronGuard() {
	tests := []struct {
		name        string
		pendingType types.ModifySubscriptionType
		wantSkipped bool
	}{
		{"pending trial end checkout defers the cron", types.ModifySubscriptionTypeTrialEnd, true},
		{"pending quantity change checkout does not block the cron", types.ModifySubscriptionTypeQuantityChange, false},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			ctx := s.GetContext()
			sub := s.seedTrialingFixedFeeSubscription(decimal.NewFromInt(200))
			s.seedPendingModifyCheckout(sub, tt.pendingType)

			subSvc := s.service.(*subscriptionService)
			inv, err := subSvc.ProcessSingleSubscriptionTrialEnd(ctx, sub, sub.TrialEnd.Add(time.Hour))
			s.Require().NoError(err)
			got, err := s.GetStores().SubscriptionRepo.Get(ctx, sub.ID)
			s.Require().NoError(err)
			if tt.wantSkipped {
				s.Nil(inv, "cron must not raise a second trial-end invoice while checkout is pending")
				s.Equal(types.SubscriptionStatusTrialing, got.SubscriptionStatus)
				return
			}
			s.NotNil(inv, "an unrelated pending checkout must not freeze the natural trial end")
			s.NotEqual(types.SubscriptionStatusTrialing, got.SubscriptionStatus)
		})
	}
}

func (s *SubscriptionServiceSuite) TestTrialEndCheckout_CronSkipsWhilePending() {
	ctx := s.GetContext()
	sub := s.seedTrialingFixedFeeSubscription(decimal.NewFromInt(200))
	modSvc := NewSubscriptionModificationService(s.service.(*subscriptionService).ServiceParams)
	_, err := modSvc.Execute(ctx, sub.ID, s.trialEndCheckoutRequest())
	s.Require().NoError(err)

	subSvc := s.service.(*subscriptionService)
	inv, err := subSvc.ProcessSingleSubscriptionTrialEnd(ctx, sub, sub.TrialEnd.Add(time.Hour))
	s.Require().NoError(err)
	s.Nil(inv, "cron must not raise a second trial-end invoice while checkout is pending")
	got, _ := s.GetStores().SubscriptionRepo.Get(ctx, sub.ID)
	s.Equal(types.SubscriptionStatusTrialing, got.SubscriptionStatus)
}
