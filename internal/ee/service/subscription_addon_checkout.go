package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

func (s *subscriptionService) previewAddonAddProration(
	ctx context.Context,
	sub *subscription.Subscription,
	req *dto.AddAddonToSubscriptionRequest,
) (*LineItemProrationSummary, error) {
	if req.ProrationBehavior != types.ProrationBehaviorCreateProrations {
		return &LineItemProrationSummary{
			Currency:          sub.Currency,
			TotalChargeAmount: decimal.Zero,
			TotalCreditAmount: decimal.Zero,
		}, nil
	}

	a, err := NewAddonService(s.ServiceParams).GetAddon(ctx, req.AddonID)
	if err != nil {
		return nil, err
	}

	validPrices, err := s.ValidateAndFilterPricesForSubscription(ctx, req.AddonID, types.PRICE_ENTITY_TYPE_ADDON, sub, nil)
	if err != nil {
		return nil, err
	}

	addonRequestedStart := addonRequestedStartDate(req)
	onetimePeriodEnd, err := addonOnetimePeriodEnd(sub, req, addonRequestedStart)
	if err != nil {
		return nil, err
	}

	previewAssociationID := types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ADDON_ASSOCIATION)
	lineItems, _, err := s.buildAddonLineItems(
		ctx, sub, req, validPrices, a.Addon.Name, previewAssociationID, addonRequestedStart, onetimePeriodEnd,
	)
	if err != nil {
		return nil, err
	}

	entries, err := s.buildAddonProrationEntries(ctx, lineItems)
	if err != nil {
		return nil, err
	}

	return NewLineItemProrationService(s.ServiceParams).Compute(ctx, LineItemProrationRequest{
		Subscription:  sub,
		Entries:       entries,
		EffectiveDate: addonProrationEffectiveDate(addonRequestedStart, lineItems),
		Behavior:      req.ProrationBehavior,
	})
}
