package service

import (
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/addon"
	"github.com/flexprice/flexprice/internal/domain/addonassociation"
	"github.com/flexprice/flexprice/internal/domain/entitlement"
	"github.com/flexprice/flexprice/internal/domain/feature"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// razorpayCheckoutParams is the minimal checkout object the attach endpoint accepts.
func (s *SubscriptionServiceSuite) razorpayCheckoutParams() *dto.CheckoutParams {
	return &dto.CheckoutParams{
		PaymentParams: dto.PaymentParams{
			PaymentProvider: types.CheckoutPaymentProviderRazorpay,
		},
	}
}

// seedPendingAddonAssociation writes a pending association straight through the store, the
// state the payment-gated attach flow will produce once it exists. The in-memory store does
// no status normalization, so the row lands exactly as written.
func (s *SubscriptionServiceSuite) seedPendingAddonAssociation(
	associationID, addonID, subscriptionID string,
	endDate *time.Time,
) *addonassociation.AddonAssociation {
	ctx := s.GetContext()
	start := s.testData.now

	assoc := &addonassociation.AddonAssociation{
		ID:          associationID,
		EntityID:    subscriptionID,
		EntityType:  types.AddonAssociationEntityTypeSubscription,
		AddonID:     addonID,
		StartDate:   &start,
		EndDate:     endDate,
		AddonStatus: types.AddonStatusPending,
		BaseModel:   types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().AddonAssociationRepo.Create(ctx, assoc))

	return assoc
}

// seedFixedPriceAddon registers a published addon with a single FIXED price, the shape that
// actually produces a proration charge when attached mid-period.
func (s *SubscriptionServiceSuite) seedFixedPriceAddon(
	addonID string,
	amount decimal.Decimal,
	cadence types.InvoiceCadence,
) {
	ctx := s.GetContext()

	s.NoError(s.GetStores().AddonRepo.Create(ctx, &addon.Addon{
		ID:        addonID,
		LookupKey: addonID,
		Name:      "Fixed Price Addon",
		BaseModel: types.GetDefaultBaseModel(ctx),
	}))

	s.NoError(s.GetStores().PriceRepo.Create(ctx, &price.Price{
		ID:                 "price_" + addonID,
		Amount:             amount,
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_ADDON,
		EntityID:           addonID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		InvoiceCadence:     cadence,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}))
}

// oneOffInvoicesFor returns the subscription's ONE_OFF invoices — the shape addon proration
// charges take.
func (s *SubscriptionServiceSuite) oneOffInvoicesFor(subscriptionID string) []*invoice.Invoice {
	filter := types.NewNoLimitInvoiceFilter()
	filter.SubscriptionID = subscriptionID
	invoices, err := s.GetStores().InvoiceRepo.List(s.GetContext(), filter)
	s.NoError(err)

	oneOff := make([]*invoice.Invoice, 0, len(invoices))
	for _, inv := range invoices {
		if inv.InvoiceType == types.InvoiceTypeOneOff {
			oneOff = append(oneOff, inv)
		}
	}
	return oneOff
}

func (s *SubscriptionServiceSuite) addonLineItemsFor(subscriptionID, addonID string) []*subscription.SubscriptionLineItem {
	filter := types.NewNoLimitSubscriptionLineItemFilter()
	filter.SubscriptionIDs = []string{subscriptionID}
	filter.EntityIDs = []string{addonID}
	filter.EntityType = lo.ToPtr(types.SubscriptionLineItemEntityTypeAddon)

	items, err := s.GetStores().SubscriptionLineItemRepo.List(s.GetContext(), filter)
	s.NoError(err)
	return items
}

func (s *SubscriptionServiceSuite) checkoutSessionCount() int {
	sessions, err := s.GetStores().CheckoutSessionRepo.List(s.GetContext(), &types.CheckoutSessionFilter{
		QueryFilter: types.NewNoLimitPublishedQueryFilter(),
	})
	s.NoError(err)
	return len(sessions)
}

// seedMeteredAddon registers a published addon with one usage price and one metered
// entitlement on featureID at the given reset period.
func (s *SubscriptionServiceSuite) seedMeteredAddon(
	addonID, featureID string,
	resetPeriod types.EntitlementUsageResetPeriod,
) {
	ctx := s.GetContext()

	s.NoError(s.GetStores().AddonRepo.Create(ctx, &addon.Addon{
		ID:        addonID,
		LookupKey: addonID,
		Name:      "Metered Addon",
		BaseModel: types.GetDefaultBaseModel(ctx),
	}))

	s.NoError(s.GetStores().PriceRepo.Create(ctx, &price.Price{
		ID:                 "price_" + addonID,
		Amount:             decimal.Zero,
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_ADDON,
		EntityID:           addonID,
		Type:               types.PRICE_TYPE_USAGE,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		InvoiceCadence:     types.InvoiceCadenceArrear,
		MeterID:            s.testData.meters.apiCalls.ID,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}))

	_, err := s.GetStores().EntitlementRepo.Create(ctx, &entitlement.Entitlement{
		ID:               "ent_" + addonID,
		EntityType:       types.ENTITLEMENT_ENTITY_TYPE_ADDON,
		EntityID:         addonID,
		FeatureID:        featureID,
		FeatureType:      types.FeatureTypeMetered,
		IsEnabled:        true,
		UsageLimit:       lo.ToPtr(int64(1000)),
		UsageResetPeriod: resetPeriod,
		BaseModel:        types.GetDefaultBaseModel(ctx),
	})
	s.NoError(err)
}

// A pending association is gated behind an unpaid checkout: it owns no line items, credit
// grants or entitlements, so removing it through the addon API would strand the session that
// is meant to activate it. RemoveAddonFromSubscription reads via GetByID, which applies no
// addon_status filter, so the association is reachable and the guard has to be explicit.
func (s *SubscriptionServiceSuite) TestRemoveAddon_RejectsPendingAssociation() {
	ctx := s.GetContext()
	sub := s.testData.subscription

	assoc := s.seedPendingAddonAssociation("assoc_pending_remove", "addon_pending_remove", sub.ID, nil)

	err := s.service.RemoveAddonFromSubscription(ctx, &dto.RemoveAddonRequest{
		AddonAssociationID: assoc.ID,
	})

	s.Error(err)
	s.True(ierr.IsValidation(err))
	s.Contains(err.Error(), "pending payment")

	stored, getErr := s.GetStores().AddonAssociationRepo.GetByID(ctx, assoc.ID)
	s.NoError(getErr)
	s.Equal(types.AddonStatusPending, stored.AddonStatus, "rejected removal must leave the association untouched")
	s.Nil(stored.CancelledAt)
}

// A onetime pending association carries the cadence boundary as its EndDate. The pending
// guard must be reached before the "already scheduled to be removed" EndDate check, or the
// caller is told the wrong reason and never learns a checkout is outstanding.
func (s *SubscriptionServiceSuite) TestRemoveAddon_RejectsPendingOnetimeAssociationWithEndDate() {
	ctx := s.GetContext()
	sub := s.testData.subscription

	periodEnd := sub.CurrentPeriodEnd
	assoc := s.seedPendingAddonAssociation("assoc_pending_onetime", "addon_pending_onetime", sub.ID, &periodEnd)

	err := s.service.RemoveAddonFromSubscription(ctx, &dto.RemoveAddonRequest{
		AddonAssociationID: assoc.ID,
	})

	s.Error(err)
	s.Contains(err.Error(), "pending payment")
	s.NotContains(err.Error(), "already scheduled to be removed")
}

// Cancelling a subscription must take its pending associations with it. They are invisible to
// the active-window read cancelAddonsForSubscription uses, so without the explicit pending
// read they outlive the subscription as orphans that a late checkout could still activate.
func (s *SubscriptionServiceSuite) TestCancelSubscription_CancelsPendingAssociations() {
	ctx := s.GetContext()

	sub := &subscription.Subscription{
		ID:                 "sub_cancel_pending_addon",
		CustomerID:         s.testData.customer.ID,
		PlanID:             s.testData.plan.ID,
		SubscriptionStatus: types.SubscriptionStatusActive,
		StartDate:          s.testData.now.Add(-30 * 24 * time.Hour),
		CurrentPeriodStart: s.testData.now.Add(-24 * time.Hour),
		CurrentPeriodEnd:   s.testData.now.Add(6 * 24 * time.Hour),
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		Currency:           "usd",
		BaseModel:          types.GetDefaultBaseModel(ctx),
		LineItems:          []*subscription.SubscriptionLineItem{},
	}
	s.NoError(s.GetStores().SubscriptionRepo.CreateWithLineItems(ctx, sub, sub.LineItems))

	assoc := s.seedPendingAddonAssociation("assoc_pending_cancel", "addon_pending_cancel", sub.ID, nil)

	_, err := s.service.CancelSubscription(ctx, sub.ID, &dto.CancelSubscriptionRequest{
		CancellationType: types.CancellationTypeImmediate,
		Reason:           "test cancellation",
	})
	s.NoError(err)

	stored, err := s.GetStores().AddonAssociationRepo.GetByID(ctx, assoc.ID)
	s.NoError(err)
	s.Equal(types.AddonStatusCancelled, stored.AddonStatus,
		"pending association must be cancelled alongside the subscription")
	s.NotNil(stored.CancelledAt)
	s.NotNil(stored.EndDate)
}

// A onetime pending association's EndDate is the cadence boundary, not a removal schedule, so
// the "already scheduled for removal, skip" branch must not swallow it during cancellation.
func (s *SubscriptionServiceSuite) TestCancelSubscription_CancelsPendingOnetimeAssociationWithEndDate() {
	ctx := s.GetContext()

	sub := &subscription.Subscription{
		ID:                 "sub_cancel_pending_onetime",
		CustomerID:         s.testData.customer.ID,
		PlanID:             s.testData.plan.ID,
		SubscriptionStatus: types.SubscriptionStatusActive,
		StartDate:          s.testData.now.Add(-30 * 24 * time.Hour),
		CurrentPeriodStart: s.testData.now.Add(-24 * time.Hour),
		CurrentPeriodEnd:   s.testData.now.Add(6 * 24 * time.Hour),
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		Currency:           "usd",
		BaseModel:          types.GetDefaultBaseModel(ctx),
		LineItems:          []*subscription.SubscriptionLineItem{},
	}
	s.NoError(s.GetStores().SubscriptionRepo.CreateWithLineItems(ctx, sub, sub.LineItems))

	periodEnd := sub.CurrentPeriodEnd
	assoc := s.seedPendingAddonAssociation("assoc_pending_cancel_onetime", "addon_pending_cancel_onetime", sub.ID, &periodEnd)

	_, err := s.service.CancelSubscription(ctx, sub.ID, &dto.CancelSubscriptionRequest{
		CancellationType: types.CancellationTypeImmediate,
	})
	s.NoError(err)

	stored, err := s.GetStores().AddonAssociationRepo.GetByID(ctx, assoc.ID)
	s.NoError(err)
	s.Equal(types.AddonStatusCancelled, stored.AddonStatus)
}

// The concurrent guard only runs inside the payment-gated path, so a plain pay-later add slips
// past it. Pending addon A grants a metered feature at MONTHLY; adding addon B granting the
// same feature at ANNUAL must be rejected, otherwise A's checkout completing leaves two
// conflicting reset periods live on one feature — the state this validation exists to prevent.
func (s *SubscriptionServiceSuite) TestValidateEntitlementCompatibility_SeesPendingAssociation() {
	ctx := s.GetContext()
	subService := s.service.(*subscriptionService)
	sub := s.testData.subscription

	featureID := "feat_pending_conflict"
	s.NoError(s.GetStores().FeatureRepo.Create(ctx, &feature.Feature{
		ID:        featureID,
		Name:      "Shared Metered Feature",
		Type:      types.FeatureTypeMetered,
		MeterID:   s.testData.meters.apiCalls.ID,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}))

	pendingAddonID := "addon_pending_monthly"
	incomingAddonID := "addon_incoming_annual"
	s.seedMeteredAddon(pendingAddonID, featureID, types.ENTITLEMENT_USAGE_RESET_PERIOD_MONTHLY)
	s.seedMeteredAddon(incomingAddonID, featureID, types.ENTITLEMENT_USAGE_RESET_PERIOD_ANNUAL)

	// Without the pending association the conflicting add is accepted: the pending addon's
	// entitlements are invisible to GetSubscriptionEntitlements by design.
	s.NoError(subService.validateEntitlementCompatibility(ctx, sub.ID, incomingAddonID),
		"baseline: nothing to conflict with before the pending association exists")

	s.seedPendingAddonAssociation("assoc_pending_entitlement", pendingAddonID, sub.ID, nil)

	err := subService.validateEntitlementCompatibility(ctx, sub.ID, incomingAddonID)
	s.Error(err)
	s.True(ierr.IsValidation(err))
	s.Contains(err.Error(), "reset period")

	// The same addon's own reset period must still pass — the guard rejects conflicts, not
	// every feature a pending addon happens to touch.
	matchingAddonID := "addon_incoming_monthly"
	s.seedMeteredAddon(matchingAddonID, featureID, types.ENTITLEMENT_USAGE_RESET_PERIOD_MONTHLY)
	s.NoError(subService.validateEntitlementCompatibility(ctx, sub.ID, matchingAddonID))
}

// Baseline for everything the checkout path must not disturb. Attaching at the period start
// makes the proration a full period, so the charge is exactly the price and the money assertion
// is deterministic rather than a tolerance.
func (s *SubscriptionServiceSuite) TestAddAddon_NoCheckout_PayLaterUnchanged() {
	ctx := s.GetContext()
	sub := s.testData.subscription
	addonID := "addon_paylater_fixed"

	amount := decimal.NewFromInt(30)
	s.seedFixedPriceAddon(addonID, amount, types.InvoiceCadenceAdvance)

	periodStart := sub.CurrentPeriodStart
	resp, err := s.service.AddAddonToSubscription(ctx, &dto.AddAddonRequest{
		SubscriptionID: sub.ID,
		AddAddonToSubscriptionRequest: dto.AddAddonToSubscriptionRequest{
			AddonID:           addonID,
			Cadence:           types.AddonCadenceRecurring,
			StartDate:         &periodStart,
			ProrationBehavior: types.ProrationBehaviorCreateProrations,
		},
	})
	s.Require().NoError(err)

	s.Require().NotNil(resp.AddonAssociation)
	s.Equal(types.AddonStatusActive, resp.AddonAssociation.AddonStatus)
	s.Nil(resp.CheckoutSession, "pay-later must not produce a checkout session")
	s.Nil(resp.Invoice)
	s.Zero(s.checkoutSessionCount())

	s.Len(s.addonLineItemsFor(sub.ID, addonID), 1)

	invoices := s.oneOffInvoicesFor(sub.ID)
	s.Require().Len(invoices, 1, "pay-later must raise exactly one ONE_OFF proration invoice")
	s.Equal(string(types.InvoiceBillingReasonSubscriptionUpdate), invoices[0].BillingReason)
	s.True(amount.Equal(invoices[0].AmountDue),
		"full-period attach must charge the full price, got %s", invoices[0].AmountDue)
}

// checkout + a net charge is the Phase 4 surface; until it lands the request must be refused
// outright rather than silently attaching and billing pay-later.
func (s *SubscriptionServiceSuite) TestAddAddon_CheckoutNetCharge_NotYetSupported() {
	ctx := s.GetContext()
	sub := s.testData.subscription
	addonID := "addon_checkout_charge"

	s.seedFixedPriceAddon(addonID, decimal.NewFromInt(30), types.InvoiceCadenceAdvance)

	now := s.testData.now
	_, err := s.service.AddAddonToSubscription(ctx, &dto.AddAddonRequest{
		SubscriptionID: sub.ID,
		Checkout:       s.razorpayCheckoutParams(),
		AddAddonToSubscriptionRequest: dto.AddAddonToSubscriptionRequest{
			AddonID:           addonID,
			Cadence:           types.AddonCadenceRecurring,
			StartDate:         &now,
			ProrationBehavior: types.ProrationBehaviorCreateProrations,
		},
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "not yet supported")

	s.Empty(s.addonLineItemsFor(sub.ID, addonID), "nothing may be persisted on the refused path")
	s.Empty(s.oneOffInvoicesFor(sub.ID))
	s.Zero(s.checkoutSessionCount())
}

// Every route to a zero net: the pay-later path raises no charge for these, so gating them
// behind payment would ask the customer to pay nothing. They attach immediately and the
// checkout is ignored.
func (s *SubscriptionServiceSuite) TestAddAddon_CheckoutZeroNet_AttachesImmediately() {
	tests := []struct {
		name              string
		usagePrice        bool
		prorationBehavior types.ProrationBehavior
	}{
		{
			// calculator.go short-circuits on `none` before computing anything.
			name:              "proration behavior none",
			prorationBehavior: types.ProrationBehaviorNone,
		},
		{
			// D2: Compute would price this, but Apply is a no-op for anything other than
			// create_prorations — so previewing a charge here would diverge from pay-later.
			name:              "proration behavior unset",
			prorationBehavior: "",
		},
		{
			// Future consumption is unknown at change time, so usage prices are skipped.
			name:              "usage only addon prices",
			usagePrice:        true,
			prorationBehavior: types.ProrationBehaviorCreateProrations,
		},
	}

	for i, tc := range tests {
		s.Run(tc.name, func() {
			ctx := s.GetContext()
			sub := s.testData.subscription
			addonID := fmt.Sprintf("addon_zero_net_%d", i)

			if tc.usagePrice {
				s.seedMeteredAddon(addonID, "", types.ENTITLEMENT_USAGE_RESET_PERIOD_MONTHLY)
			} else {
				s.seedFixedPriceAddon(addonID, decimal.NewFromInt(30), types.InvoiceCadenceAdvance)
			}

			now := s.testData.now
			resp, err := s.service.AddAddonToSubscription(ctx, &dto.AddAddonRequest{
				SubscriptionID: sub.ID,
				Checkout:       s.razorpayCheckoutParams(),
				AddAddonToSubscriptionRequest: dto.AddAddonToSubscriptionRequest{
					AddonID:           addonID,
					Cadence:           types.AddonCadenceRecurring,
					StartDate:         &now,
					ProrationBehavior: tc.prorationBehavior,
				},
			})
			s.Require().NoError(err)

			s.Require().NotNil(resp.AddonAssociation)
			s.Equal(types.AddonStatusActive, resp.AddonAssociation.AddonStatus,
				"a zero-net attach takes effect immediately")
			s.Nil(resp.CheckoutSession, "checkout must be ignored when there is nothing to charge")
			s.Len(s.addonLineItemsFor(sub.ID, addonID), 1)
			s.Zero(s.checkoutSessionCount())
		})
	}
}

// The v1 checkout path cannot honour overrides, commitments or draft subscriptions. Each must
// be refused before anything is written.
func (s *SubscriptionServiceSuite) TestAddAddon_CheckoutRejectsUnsupportedCombinations() {
	ctx := s.GetContext()
	sub := s.testData.subscription

	s.Run("override_line_items", func() {
		addonID := "addon_reject_override"
		s.seedFixedPriceAddon(addonID, decimal.NewFromInt(30), types.InvoiceCadenceAdvance)

		now := s.testData.now
		_, err := s.service.AddAddonToSubscription(ctx, &dto.AddAddonRequest{
			SubscriptionID: sub.ID,
			Checkout:       s.razorpayCheckoutParams(),
			AddAddonToSubscriptionRequest: dto.AddAddonToSubscriptionRequest{
				AddonID:           addonID,
				Cadence:           types.AddonCadenceRecurring,
				StartDate:         &now,
				ProrationBehavior: types.ProrationBehaviorCreateProrations,
				OverrideLineItems: []dto.OverrideLineItemRequest{{
					PriceID:  "price_" + addonID,
					Quantity: lo.ToPtr(decimal.NewFromInt(2)),
				}},
			},
		})

		s.Require().Error(err)
		s.True(ierr.IsValidation(err))
		s.Empty(s.addonLineItemsFor(sub.ID, addonID))
		s.Zero(s.checkoutSessionCount())
	})

	s.Run("line_item_commitments", func() {
		addonID := "addon_reject_commitment"
		s.seedFixedPriceAddon(addonID, decimal.NewFromInt(30), types.InvoiceCadenceAdvance)

		now := s.testData.now
		_, err := s.service.AddAddonToSubscription(ctx, &dto.AddAddonRequest{
			SubscriptionID: sub.ID,
			Checkout:       s.razorpayCheckoutParams(),
			AddAddonToSubscriptionRequest: dto.AddAddonToSubscriptionRequest{
				AddonID:           addonID,
				Cadence:           types.AddonCadenceRecurring,
				StartDate:         &now,
				ProrationBehavior: types.ProrationBehaviorCreateProrations,
				LineItemCommitments: map[string]*dto.LineItemCommitmentConfig{
					"price_" + addonID: {CommitmentAmount: lo.ToPtr(decimal.NewFromInt(10))},
				},
			},
		})

		s.Require().Error(err)
		s.True(ierr.IsValidation(err))
		s.Empty(s.addonLineItemsFor(sub.ID, addonID))
		s.Zero(s.checkoutSessionCount())
	})

	s.Run("draft subscription", func() {
		addonID := "addon_reject_draft"
		s.seedFixedPriceAddon(addonID, decimal.NewFromInt(30), types.InvoiceCadenceAdvance)

		draft := &subscription.Subscription{
			ID:                 "sub_draft_checkout_addon",
			CustomerID:         s.testData.customer.ID,
			PlanID:             s.testData.plan.ID,
			SubscriptionStatus: types.SubscriptionStatusDraft,
			StartDate:          s.testData.now,
			CurrentPeriodStart: s.testData.now,
			CurrentPeriodEnd:   s.testData.now.AddDate(0, 1, 0),
			BillingAnchor:      s.testData.now,
			BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
			BillingPeriodCount: 1,
			Currency:           "usd",
			BaseModel:          types.GetDefaultBaseModel(ctx),
			LineItems:          []*subscription.SubscriptionLineItem{},
		}
		s.NoError(s.GetStores().SubscriptionRepo.CreateWithLineItems(ctx, draft, draft.LineItems))

		now := s.testData.now
		_, err := s.service.AddAddonToSubscription(ctx, &dto.AddAddonRequest{
			SubscriptionID: draft.ID,
			Checkout:       s.razorpayCheckoutParams(),
			AddAddonToSubscriptionRequest: dto.AddAddonToSubscriptionRequest{
				AddonID:           addonID,
				Cadence:           types.AddonCadenceRecurring,
				StartDate:         &now,
				ProrationBehavior: types.ProrationBehaviorCreateProrations,
			},
		})

		s.Require().Error(err)
		s.True(ierr.IsValidation(err))
		s.Contains(err.Error(), "status does not allow")
		s.Empty(s.addonLineItemsFor(draft.ID, addonID))
		s.Zero(s.checkoutSessionCount())

		// Draft attach without checkout stays supported.
		_, err = s.service.AddAddonToSubscription(ctx, &dto.AddAddonRequest{
			SubscriptionID: draft.ID,
			AddAddonToSubscriptionRequest: dto.AddAddonToSubscriptionRequest{
				AddonID:   addonID,
				Cadence:   types.AddonCadenceRecurring,
				StartDate: &now,
			},
		})
		s.NoError(err, "pay-later attach to a draft subscription must be unaffected")
	})
}

// Pending associations must stay out of the reads that grant real access and drive the public
// addon list — they are only visible to the compatibility check and the cancellation sweep.
func (s *SubscriptionServiceSuite) TestPendingAssociation_InvisibleToActiveReads() {
	ctx := s.GetContext()
	subService := s.service.(*subscriptionService)
	sub := s.testData.subscription

	featureID := "feat_pending_invisible"
	s.NoError(s.GetStores().FeatureRepo.Create(ctx, &feature.Feature{
		ID:        featureID,
		Name:      "Pending Feature",
		Type:      types.FeatureTypeMetered,
		MeterID:   s.testData.meters.apiCalls.ID,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}))

	addonID := "addon_pending_invisible"
	s.seedMeteredAddon(addonID, featureID, types.ENTITLEMENT_USAGE_RESET_PERIOD_MONTHLY)
	s.seedPendingAddonAssociation("assoc_pending_invisible", addonID, sub.ID, nil)

	associations, err := subService.GetActiveAddonAssociations(ctx, sub.ID)
	s.NoError(err)
	for _, item := range associations.Items {
		s.NotEqual(addonID, item.AddonAssociation.AddonID,
			"pending association must not surface in the public addon associations list")
	}

	entitlements, err := subService.GetSubscriptionEntitlements(ctx, sub.ID)
	s.NoError(err)
	for _, ent := range entitlements {
		s.NotEqual(featureID, ent.FeatureID,
			"pending association must not grant entitlements before its checkout completes")
	}
}
