package selfbilling

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/price"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func (c *remoteClient) summaryPlan(ctx context.Context, sub *remoteSubscription) (*dto.SelfBillingPlanResponse, error) {
	if sub == nil || sub.status == types.SubscriptionStatusCancelled {
		resp, err := c.sdk.Plans.GetPlan(ctx, c.planID)
		if err != nil {
			return nil, cloudErr(err)
		}
		if resp.PlanResponse == nil {
			return nil, unreadablePricing()
		}
		var plan dto.PlanResponse
		if err := decodePricing(resp.PlanResponse, &plan); err != nil {
			return nil, err
		}
		return offeredPlan(&plan, time.Now().UTC())
	}

	resp, err := c.sdk.Subscriptions.GetSubscriptionV2(ctx, sub.id, lo.ToPtr("plan,subscription_line_items.prices"))
	if err != nil {
		return nil, cloudErr(err)
	}
	if resp.SubscriptionResponseV2 == nil {
		return nil, unreadablePricing()
	}
	var subscription dto.SubscriptionResponseV2
	if err := decodePricing(resp.SubscriptionResponseV2, &subscription); err != nil {
		return nil, err
	}
	if subscription.Subscription == nil || subscription.ID != sub.id || subscription.SubscriptionStatus == "" {
		return nil, unreadablePricing()
	}
	// Reconcile status from the same snapshot as pricing when checkout raced the subscription search.
	sub.status, sub.trialEnd = subscription.SubscriptionStatus, subscription.TrialEnd
	if sub.status == types.SubscriptionStatusCancelled {
		return c.summaryPlan(ctx, sub)
	}
	return subscribedPlan(&subscription, time.Now().UTC())
}

func offeredPlan(plan *dto.PlanResponse, now time.Time) (*dto.SelfBillingPlanResponse, error) {
	if plan.Plan == nil || plan.ID == "" {
		return nil, unreadablePricing()
	}
	out := newSummaryPlan(plan)
	for _, p := range plan.Prices {
		if p == nil || p.Price == nil {
			return nil, unreadablePricing()
		}
		if !p.IsActive(&now) || !strings.EqualFold(p.Currency, trialCurrency) {
			continue
		}
		if p.BillingPeriod != types.BillingPeriod(trialBillingPeriod) && p.BillingPeriod != types.BILLING_PERIOD_ONETIME {
			continue
		}
		out.Charges = append(out.Charges, summaryCharge(p, price.ApplyQuantityDefault(decimal.Zero, p.Price)))
	}
	return out, nil
}

func subscribedPlan(sub *dto.SubscriptionResponseV2, now time.Time) (*dto.SelfBillingPlanResponse, error) {
	if sub.Subscription == nil || sub.Plan == nil || sub.Plan.Plan == nil || sub.Plan.ID == "" {
		return nil, unreadablePricing()
	}
	out := newSummaryPlan(sub.Plan)
	if sub.CommitmentAmount != nil {
		out.Commitment = &dto.SelfBillingCommitment{
			Amount: sub.CommitmentAmount, Duration: sub.CommitmentDuration,
			OverageFactor: sub.OverageFactor, TrueUpEnabled: sub.EnableTrueUp,
		}
	}
	for _, item := range sub.LineItems {
		if item == nil || item.SubscriptionLineItem == nil {
			return nil, unreadablePricing()
		}
		if !item.IsActive(now) {
			continue
		}
		if item.Price == nil || item.Price.Price == nil || item.Price.ID == "" || item.Price.ID != item.PriceID {
			return nil, unreadablePricing()
		}
		// A grandfathered line can still reference an expired catalog price.
		charge := summaryCharge(item.Price, item.Quantity)
		charge.LineItemID = item.ID
		if item.DisplayName != "" {
			charge.DisplayName = item.DisplayName
		}
		charge.Currency = item.Currency
		charge.BillingPeriod = item.BillingPeriod
		charge.BillingPeriodCount = item.BillingPeriodCount
		charge.InvoiceCadence = item.InvoiceCadence
		if item.MeterDisplayName != "" {
			charge.MeterDisplayName = item.MeterDisplayName
		}
		if item.CommitmentAmount != nil || item.CommitmentQuantity != nil || len(item.CommitmentTimeBuckets) > 0 {
			charge.Commitment = &dto.SelfBillingCommitment{
				Amount: item.CommitmentAmount, Quantity: item.CommitmentQuantity, Type: item.CommitmentType,
				Duration: item.CommitmentDuration, OverageFactor: item.CommitmentOverageFactor,
				TrueUpEnabled: item.CommitmentTrueUpEnabled, Windowed: item.CommitmentWindowed,
				TimeBuckets: item.CommitmentTimeBuckets,
			}
		}
		out.Charges = append(out.Charges, charge)
	}
	return out, nil
}

func newSummaryPlan(plan *dto.PlanResponse) *dto.SelfBillingPlanResponse {
	return &dto.SelfBillingPlanResponse{
		ID: plan.ID, Name: plan.Name, Description: plan.Description,
		Charges: []dto.SelfBillingChargeResponse{},
	}
}

func summaryCharge(p *dto.PriceResponse, quantity decimal.Decimal) dto.SelfBillingChargeResponse {
	charge := dto.SelfBillingChargeResponse{
		PriceID: p.ID, DisplayName: p.DisplayName, Type: p.Type,
		Quantity: quantity, Amount: p.Amount, Currency: p.Currency,
		BillingModel: p.BillingModel, BillingCadence: p.BillingCadence,
		BillingPeriod: p.BillingPeriod, BillingPeriodCount: p.BillingPeriodCount, InvoiceCadence: p.InvoiceCadence,
		TierMode: p.TierMode, Tiers: p.Tiers, TransformQuantity: price.TransformQuantity(p.TransformQuantity),
		MinQuantity: p.MinQuantity, MeterID: p.MeterID, BucketSize: p.BucketSize,
		PriceUnitType: p.PriceUnitType, PriceUnit: p.PriceUnit, PriceUnitAmount: p.PriceUnitAmount,
		PriceUnitTiers: p.PriceUnitTiers, ConversionRate: p.ConversionRate,
	}
	if p.Meter != nil {
		charge.MeterDisplayName = p.Meter.Name
	}
	return charge
}

// Flexprice Cloud serves the same wire types; decode decimals before selecting the public pricing fields.
func decodePricing(in, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return ierr.WithError(err).WithHint("Flexprice Cloud returned unreadable pricing").Mark(ierr.ErrInternal)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return ierr.WithError(err).WithHint("Flexprice Cloud returned unreadable pricing").Mark(ierr.ErrInternal)
	}
	return nil
}

func unreadablePricing() error {
	return ierr.NewError("flexprice cloud returned incomplete billing details").
		WithHint("Flexprice Cloud returned unreadable pricing").Mark(ierr.ErrInternal)
}
