package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// finishGatedSubscriptionActivation runs the post-activation work a normal create does inline but a
// payment-gated create cannot, because at create time the subscription is still DRAFT and unpaid:
// pending credit-grant applications are processed and subscription.created is published.
//
// Neither happened for checkout-created subscriptions before — they only ever emitted
// subscription.draft_created, and their grants waited for the credit-grant cron.
//
// Grant failures are logged rather than returned: the payment has already been collected by the
// time this runs, so failing the completion would be worse than the cron picking them up late.
func (s *subscriptionService) finishGatedSubscriptionActivation(ctx context.Context, sub *subscription.Subscription) {
	if sub == nil {
		return
	}

	if err := s.processPendingCreditGrantsForSubscription(ctx, sub); err != nil {
		s.Logger.Error(ctx, "failed to process pending credit grants for checkout-activated subscription",
			"error", err,
			"subscription_id", sub.ID,
		)
	}

	s.publishSubscriptionCreatedEvent(ctx, sub)
}

// archiveDraftCheckoutSubscription rolls back a DRAFT subscription that was materialized for a
// checkout which never completed, along with the children the full create surface can attach.
//
// Guarded on draft status so a session that completed and only later expired never touches a live
// subscription — the same shape as the addon cleanup's pending-status guard.
//
// Best-effort throughout: every failure is logged and the next entity is still attempted, because
// the caller is already on an error or expiry path and has nothing better to do with the error.
func (s *subscriptionService) archiveDraftCheckoutSubscription(ctx context.Context, subscriptionID string) {
	if subscriptionID == "" {
		return
	}

	sub, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		s.Logger.Error(ctx, "failed to load subscription for checkout cleanup",
			"error", err,
			"subscription_id", subscriptionID,
		)
		return
	}

	if sub.SubscriptionStatus != types.SubscriptionStatusDraft {
		s.Logger.Info(ctx, "checkout subscription is no longer draft, skipping archival",
			"subscription_id", subscriptionID,
			"subscription_status", sub.SubscriptionStatus,
		)
		return
	}

	s.archiveDraftSubscriptionChildren(ctx, subscriptionID)

	if err := s.SubRepo.Delete(ctx, subscriptionID); err != nil {
		s.Logger.Error(ctx, "failed to archive draft checkout subscription",
			"error", err,
			"subscription_id", subscriptionID,
		)
	}
}

// archiveDraftSubscriptionChildren archives the entities createSubscription attaches to a DRAFT
// subscription. Line items are not archived here — billing reaches them only through a non-draft
// subscription, so archiving the parent is enough.
//
// Note: coupon redemptions incremented by createCouponAssociation are NOT given back. There is no
// decrement on the coupon repository, and the same leak exists for every other path that abandons a
// subscription carrying coupons.
func (s *subscriptionService) archiveDraftSubscriptionChildren(ctx context.Context, subscriptionID string) {
	addonFilter := types.NewNoLimitAddonAssociationFilter()
	addonFilter.EntityType = lo.ToPtr(types.AddonAssociationEntityTypeSubscription)
	addonFilter.EntityIDs = []string{subscriptionID}
	if associations, err := s.AddonAssociationRepo.List(ctx, addonFilter); err != nil {
		s.Logger.Error(ctx, "failed to list addon associations for draft subscription cleanup",
			"error", err, "subscription_id", subscriptionID)
	} else {
		for _, association := range associations {
			if err := s.AddonAssociationRepo.Delete(ctx, association.ID); err != nil {
				s.Logger.Error(ctx, "failed to archive addon association for draft subscription",
					"error", err, "subscription_id", subscriptionID, "association_id", association.ID)
			}
		}
	}

	couponFilter := &types.CouponAssociationFilter{
		QueryFilter:     types.NewNoLimitQueryFilter(),
		SubscriptionIDs: []string{subscriptionID},
	}
	if associations, err := s.CouponAssociationRepo.List(ctx, couponFilter); err != nil {
		s.Logger.Error(ctx, "failed to list coupon associations for draft subscription cleanup",
			"error", err, "subscription_id", subscriptionID)
	} else {
		for _, association := range associations {
			if err := s.CouponAssociationRepo.Delete(ctx, association.ID); err != nil {
				s.Logger.Error(ctx, "failed to archive coupon association for draft subscription",
					"error", err, "subscription_id", subscriptionID, "association_id", association.ID)
			}
		}
	}

	// Applications first: the credit-grant cron reads applications, so leaving them behind after
	// archiving their grant is the one ordering that could still credit a wallet.
	applicationFilter := types.NewNoLimitCreditGrantApplicationFilter()
	applicationFilter.SubscriptionIDs = []string{subscriptionID}
	if applications, err := s.CreditGrantApplicationRepo.List(ctx, applicationFilter); err != nil {
		s.Logger.Error(ctx, "failed to list credit grant applications for draft subscription cleanup",
			"error", err, "subscription_id", subscriptionID)
	} else {
		for _, application := range applications {
			if err := s.CreditGrantApplicationRepo.Delete(ctx, application); err != nil {
				s.Logger.Error(ctx, "failed to archive credit grant application for draft subscription",
					"error", err, "subscription_id", subscriptionID, "application_id", application.ID)
			}
		}
	}

	grantFilter := types.NewNoLimitCreditGrantFilter()
	grantFilter.SubscriptionIDs = []string{subscriptionID}
	if grants, err := s.CreditGrantRepo.List(ctx, grantFilter); err != nil {
		s.Logger.Error(ctx, "failed to list credit grants for draft subscription cleanup",
			"error", err, "subscription_id", subscriptionID)
	} else {
		for _, grant := range grants {
			if err := s.CreditGrantRepo.Delete(ctx, grant.ID); err != nil {
				s.Logger.Error(ctx, "failed to archive credit grant for draft subscription",
					"error", err, "subscription_id", subscriptionID, "credit_grant_id", grant.ID)
			}
		}
	}

	taxFilter := &types.TaxAssociationFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
		EntityType:  types.TaxRateEntityTypeSubscription,
		EntityID:    subscriptionID,
	}
	if associations, err := s.TaxAssociationRepo.List(ctx, taxFilter); err != nil {
		s.Logger.Error(ctx, "failed to list tax associations for draft subscription cleanup",
			"error", err, "subscription_id", subscriptionID)
	} else {
		for _, association := range associations {
			if err := s.TaxAssociationRepo.Delete(ctx, association); err != nil {
				s.Logger.Error(ctx, "failed to archive tax association for draft subscription",
					"error", err, "subscription_id", subscriptionID, "tax_association_id", association.ID)
			}
		}
	}
}
