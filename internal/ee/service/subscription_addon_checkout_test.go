package service

import (
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/addon"
	"github.com/flexprice/flexprice/internal/domain/addonassociation"
	"github.com/flexprice/flexprice/internal/domain/entitlement"
	"github.com/flexprice/flexprice/internal/domain/feature"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

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
