package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/addonassociation"
	domainCheckout "github.com/flexprice/flexprice/internal/domain/checkout"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// addonAttachParams is a fully-resolved addon attach that has not been written yet, so it can be
// priced before anyone decides whether to persist it. planAddonAttach builds it,
// calculateAddonProration prices it, persistAddonAttach writes it.
type addonAttachParams struct {
	subscription   *subscription.Subscription
	request        *dto.AddAddonToSubscriptionRequest
	association    *addonassociation.AddonAssociation
	lineItems      []*subscription.SubscriptionLineItem
	bucketCfgs     map[string]*dto.LineItemCommitmentConfig
	priceMap       map[string]*dto.PriceResponse
	requestedStart time.Time
	effectiveDate  time.Time

	// isReplay marks a plan whose association is an existing pending row being activated
	// rather than a new one being created.
	isReplay bool
}

func (p *addonAttachParams) getSubscription() *subscription.Subscription {
	if p == nil {
		return nil
	}
	return p.subscription
}

func (p *addonAttachParams) getRequest() *dto.AddAddonToSubscriptionRequest {
	if p == nil {
		return nil
	}
	return p.request
}

func (p *addonAttachParams) getAssociation() *addonassociation.AddonAssociation {
	if p == nil {
		return nil
	}
	return p.association
}

func (p *addonAttachParams) getLineItems() []*subscription.SubscriptionLineItem {
	if p == nil {
		return nil
	}
	return p.lineItems
}

// getRequestedStart is the resolved attach date. It must be carried into AddAddonParams so a
// completion replay rebuilds line items for the same day the draft invoice was priced for.
func (p *addonAttachParams) getRequestedStart() time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.requestedStart
}

// getEffectiveDate is the date the proration charge is priced from: the latest line item start,
// which createLineItemFromPrice may clamp past the requested start.
func (p *addonAttachParams) getEffectiveDate() time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.effectiveDate
}

// prorationIdempotencyKey is stable across retries so a duplicate charge cannot be created.
func (p *addonAttachParams) prorationIdempotencyKey() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("addon_add_%s_%d", p.association.ID, p.effectiveDate.Unix())
}

// calculateAddonProration prices a planned attach without persisting anything. The plan's line
// items are the same objects persistAddonAttach would write, so the preview and the pay-later
// charge cannot disagree.
func (s *subscriptionService) calculateAddonProration(
	ctx context.Context,
	params *addonAttachParams,
) (*LineItemProrationSummary, error) {
	sub := params.getSubscription()
	req := params.getRequest()

	if req.ProrationBehavior != types.ProrationBehaviorCreateProrations {
		return &LineItemProrationSummary{
			Currency:          sub.Currency,
			TotalChargeAmount: decimal.Zero,
			TotalCreditAmount: decimal.Zero,
		}, nil
	}

	entries, err := s.buildAddonProrationEntries(ctx, params.getLineItems())
	if err != nil {
		return nil, err
	}

	return NewLineItemProrationService(s.ServiceParams).Compute(ctx, LineItemProrationRequest{
		Subscription:  sub,
		Entries:       entries,
		EffectiveDate: params.getEffectiveDate(),
		Behavior:      req.ProrationBehavior,
	})
}

func (s *subscriptionService) settleAddAddonPayFirst(
	ctx context.Context,
	params *addonAttachParams,
	summary *LineItemProrationSummary,
	checkout *dto.CheckoutParams,
) (*dto.AddAddonToSubscriptionResponse, error) {
	if params == nil || summary == nil || checkout == nil {
		return nil, ierr.NewError("pay-first addon attach requires a plan, proration and checkout").
			Mark(ierr.ErrValidation)
	}

	sub := params.getSubscription()
	req := params.getRequest()

	checkoutParams := &types.AddAddonParams{
		SubscriptionID: sub.ID,
		Addons: []types.AddAddonRef{{
			AssociationID:     params.getAssociation().ID,
			AddonID:           req.AddonID,
			Cadence:           req.Cadence,
			ProrationBehavior: req.ProrationBehavior,
			StartDate:         params.getRequestedStart(),
		}},
	}
	if err := checkoutParams.Validate(); err != nil {
		return nil, err
	}

	existing, err := s.getAnyPendingAddonCheckoutSession(ctx, sub.CustomerID, sub.ID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return nil, ierr.NewError("a pending checkout session already exists for this subscription").
			WithHint("Complete or cancel the existing checkout before starting another payment-gated change").
			WithReportableDetails(map[string]any{
				"subscription_id":     sub.ID,
				"checkout_session_id": existing[0].ID,
			}).
			Mark(ierr.ErrAlreadyExists)
	}

	pendingAssociation := params.getAssociation()
	pendingAssociation.AddonStatus = types.AddonStatusPending
	if err := s.AddonAssociationRepo.Create(ctx, pendingAssociation); err != nil {
		return nil, err
	}

	draftInvoice, err := s.createAddonProrationDraftInvoice(ctx, params, summary)
	if err != nil {
		s.archivePendingAddonAssociation(ctx, pendingAssociation, err)
		return nil, err
	}

	// StartPayFirstCheckoutSession already archives the draft on session-create failure and
	// runs cleanupCheckoutSession on fulfil failure, so the pending association is the only
	// thing this path has to unwind itself.
	checkoutSvc := NewCheckoutSessionService(s.ServiceParams)
	sessionResp, err := checkoutSvc.StartPayFirstCheckoutSession(ctx, &dto.PayFirstCheckoutRequest{
		CustomerID: sub.CustomerID,
		Action:     types.CheckoutActionAddAddon,
		Configuration: types.CheckoutConfiguration{
			AddAddonParams: checkoutParams,
		},
		DraftInvoice: &draftInvoice.Invoice,
		Checkout:     checkout,
	})
	if err != nil {
		s.archivePendingAddonAssociation(ctx, pendingAssociation, err)
		return nil, err
	}

	// Re-fetch for the response so amounts reflect compute.
	latestInvoice, invErr := NewInvoiceService(s.ServiceParams).GetInvoice(ctx, draftInvoice.ID)
	if invErr != nil {
		latestInvoice = draftInvoice
	}

	return &dto.AddAddonToSubscriptionResponse{
		AddonAssociation: pendingAssociation,
		CheckoutSession:  sessionResp,
		Invoice:          latestInvoice,
	}, nil
}

// createAddonProrationDraftInvoice locks the charge on a DRAFT ONE_OFF using the same request
// builder the pay-later charge uses, so the amount the customer is asked to pay is exactly the
// amount pay-later would have billed.
func (s *subscriptionService) createAddonProrationDraftInvoice(
	ctx context.Context,
	params *addonAttachParams,
	summary *LineItemProrationSummary,
) (*dto.InvoiceResponse, error) {
	if !summary.TotalChargeAmount.GreaterThan(decimal.Zero) || len(summary.ChargeLineItems) == 0 {
		return nil, ierr.NewError("no proration charge to collect via checkout").
			WithHint("Expected a positive proration charge").
			Mark(ierr.ErrValidation)
	}

	req := buildLineItemProrationChargeInvoiceRequest(
		params.getSubscription(),
		summary,
		params.getEffectiveDate(),
		params.prorationIdempotencyKey(),
	)

	inv, skipped, err := NewInvoiceService(s.ServiceParams).CreateComputedDraftInvoice(ctx, req)
	if err != nil {
		s.Logger.Error(ctx, "failed to create draft proration invoice for payment-gated addon attach",
			"error", err,
			"subscription_id", params.getSubscription().ID,
			"association_id", params.getAssociation().ID,
		)
		return nil, err
	}
	if skipped {
		return nil, ierr.NewError("draft invoice was skipped").
			WithHint("Expected a non-zero invoice amount").
			WithReportableDetails(map[string]any{"invoice_id": inv.GetId()}).
			Mark(ierr.ErrValidation)
	}

	return inv, nil
}

func (s *subscriptionService) getAnyPendingAddonCheckoutSession(
	ctx context.Context,
	customerID string,
	subscriptionID string,
) ([]*domainCheckout.CheckoutSession, error) {
	filter := &types.CheckoutSessionFilter{
		QueryFilter: types.NewNoLimitPublishedQueryFilter(),
		CustomerIDs: []string{customerID},
		Actions: []types.CheckoutAction{
			types.CheckoutActionModifySubscription,
			types.CheckoutActionAddAddon,
		},
		CheckoutStatuses: []types.CheckoutStatus{
			types.CheckoutStatusInitiated,
			types.CheckoutStatusPending,
		},
		Configuration: &types.CheckoutConfigurationFilter{SubscriptionID: subscriptionID},
	}
	filter.Limit = lo.ToPtr(1)

	return s.CheckoutSessionRepo.List(ctx, filter)
}

func (s *subscriptionService) archivePendingAddonAssociation(
	ctx context.Context,
	association *addonassociation.AddonAssociation,
	cause error,
) {
	if err := s.AddonAssociationRepo.Delete(ctx, association.ID); err != nil {
		s.Logger.Error(ctx, "failed to archive pending addon association after pay-first failure",
			"error", err,
			"association_id", association.ID,
			"original_error", cause,
		)
	}
}
