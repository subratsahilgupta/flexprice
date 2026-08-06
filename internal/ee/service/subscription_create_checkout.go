package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// gateSubscriptionOnCheckout prices the DRAFT subscription createSubscription just wrote and puts
// a payment gate in front of its activation.
//
// It runs after the create rather than instead of it, which is what makes addons, coupons,
// overrides and credit grants priceable before payment — quantity change and addon attach can defer
// their whole mutation, this one cannot. The cost is that every failure here has to archive the
// draft subscription.
//
// Mutates response in place: the caller decides which lifecycle webhook to publish from the
// subscription's resulting status, so activation must be visible to it.
func (s *subscriptionService) gateSubscriptionOnCheckout(
	ctx context.Context,
	response *dto.SubscriptionResponse,
	checkout *dto.CheckoutParams,
) error {
	if checkout == nil {
		return ierr.NewError("payment-gated create requires checkout params").
			Mark(ierr.ErrValidation)
	}

	invResp, skipped, err := buildCheckoutDraftInvoice(ctx, s.ServiceParams, response)
	if err != nil {
		s.archiveDraftCheckoutSubscription(ctx, response.ID)
		return err
	}

	// ComputeInvoice's skipped flag means a zero SUBTOTAL, so it does not catch a subscription whose
	// charges are fully discounted — that one has a real subtotal and a zero amount due. Both mean
	// nothing to collect.
	if skipped || !invResp.AmountDue.GreaterThan(decimal.Zero) {
		return s.activateGatedSubscriptionNow(ctx, response, invResp, skipped)
	}

	return s.settleCreateSubscriptionPayFirst(ctx, response, invResp, checkout)
}

// activateGatedSubscriptionNow handles a gated create that turned out to owe nothing — a usage-only
// plan, or a discount that zeroes the total. No session is opened and the subscription activates
// immediately, mirroring the addon attach flow's "net <= 0 attaches immediately" branch.
//
// subscription.created is left to the caller, which publishes it for any non-draft create.
func (s *subscriptionService) activateGatedSubscriptionNow(
	ctx context.Context,
	response *dto.SubscriptionResponse,
	invResp *dto.InvoiceResponse,
	skipped bool,
) error {
	invSvc := NewInvoiceService(s.ServiceParams)
	sub := response.Subscription

	if skipped {
		// Nothing was ever computed onto it, so it would linger as an empty draft.
		if err := s.InvoiceRepo.Delete(ctx, invResp.ID); err != nil {
			s.Logger.Error(ctx, "failed to archive empty draft invoice for zero-charge checkout create",
				"error", err,
				"invoice_id", invResp.ID,
				"subscription_id", sub.ID,
			)
		}
		invResp = nil
	} else {
		// A zero-due invoice finalizes as paid, which keeps the discount on the books rather than
		// making a fully-discounted subscription look like it was never billed.
		if err := invSvc.FinalizeInvoice(ctx, invResp.ID); err != nil {
			s.archiveDraftCheckoutSubscription(ctx, sub.ID)
			return err
		}
		if refreshed, err := invSvc.GetInvoice(ctx, invResp.ID); err == nil {
			invResp = refreshed
		}
	}

	sub.SubscriptionStatus = types.SubscriptionStatusActive
	if err := s.SubRepo.Update(ctx, sub); err != nil {
		s.archiveDraftCheckoutSubscription(ctx, sub.ID)
		return err
	}

	// Grants were left pending while the subscription was draft.
	if err := s.processPendingCreditGrantsForSubscription(ctx, sub); err != nil {
		s.Logger.Error(ctx, "failed to process pending credit grants for zero-charge checkout create",
			"error", err,
			"subscription_id", sub.ID,
		)
	}

	response.LatestInvoice = invResp
	return nil
}

// settleCreateSubscriptionPayFirst opens the checkout session that gates activation. The amount is
// already locked on the draft invoice, so completion never re-prices.
func (s *subscriptionService) settleCreateSubscriptionPayFirst(
	ctx context.Context,
	response *dto.SubscriptionResponse,
	invResp *dto.InvoiceResponse,
	checkout *dto.CheckoutParams,
) error {
	sub := response.Subscription

	// The blob carries only the subscription id: everything else the completion needs already lives
	// on the row it points at.
	checkoutParams := &types.CreateSubscriptionParams{SubscriptionID: sub.ID}
	if err := checkoutParams.Validate(); err != nil {
		s.archiveDraftCheckoutSubscription(ctx, sub.ID)
		return err
	}

	checkoutSvc := NewCheckoutSessionService(s.ServiceParams)
	sessionResp, err := checkoutSvc.StartPayFirstCheckoutSession(ctx, &dto.PayFirstCheckoutRequest{
		CustomerID: sub.CustomerID,
		Action:     types.CheckoutActionCreateSubscription,
		Configuration: types.CheckoutConfiguration{
			CreateSubscriptionParams: checkoutParams,
		},
		DraftInvoice: &invResp.Invoice,
		Checkout:     checkout,
	})
	if err != nil {
		// StartPayFirstCheckoutSession archives the draft invoice and cleans up any session it
		// created; the subscription is ours to undo.
		s.archiveDraftCheckoutSubscription(ctx, sub.ID)
		return err
	}

	latestInvoice, invErr := NewInvoiceService(s.ServiceParams).GetInvoice(ctx, invResp.ID)
	if invErr != nil {
		latestInvoice = invResp
	}

	response.LatestInvoice = latestInvoice
	response.CheckoutSession = sessionResp
	return nil
}

// finishGatedSubscriptionActivation runs the post-activation work a normal create does inline:
// pending credit-grant applications are processed and subscription.created is published. Neither
// happened for checkout-created subscriptions before — they only ever emitted
// subscription.draft_created, and their grants waited for the credit-grant cron.
//
// For the WEBHOOK completion path only. The create path must not call this: CreateSubscription
// publishes the lifecycle event itself from the subscription's resulting status, so a gated create
// that activates immediately would announce itself twice.
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
