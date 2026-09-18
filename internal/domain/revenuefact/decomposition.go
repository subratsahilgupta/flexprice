package revenuefact

import (
	"time"

	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// RevenuePeriod is the billing window a period_only row is stamped against.
// Unlike the curve input's half-open [PeriodStart, PeriodEnd), End here is the
// inclusive last calendar day of the period — matching the revenue_facts
// period_end DATE column and the Day a period_only row lands on for ARREAR
// cadence. It is separate from PreviewLineItem's own period so a commitment
// true-up can be dated against a multi-period commitment window rather than
// the line item's single billing period.
type RevenuePeriod struct {
	Start time.Time
	End   time.Time
}

// ExclusiveEnd converts End (the inclusive last day) to the half-open upper
// bound expected by APIs like BuildUsageCurve.
func (p RevenuePeriod) ExclusiveEnd() time.Time {
	return p.End.AddDate(0, 0, 1)
}

// PreviewLineItem is one subscription line item as priced by the billing
// preview engine — the attached price/meter, the engine's already-computed
// charge, and the subscription-level commitment config the four Decompose*
// constructors turn into revenue_facts rows.
type PreviewLineItem struct {
	TenantID      string
	EnvironmentID string
	CustomerID    string
	// SubscriptionID is the line item's own subscription, which may differ
	// from the invoice's subscription for grouped invoicing.
	SubscriptionID string
	SubLineItemID  string

	Price *price.Price
	Meter *meter.Meter

	Currency string
	Metadata types.Metadata

	// EngineAmount is the billing engine's already-computed charge for this
	// line item over its period — used verbatim as NetAmount by every
	// period_only decomposer (fixed, usage period_only, commitment true-up).
	EngineAmount decimal.Decimal

	// PeriodStart/PeriodEnd are the line item's billing period, stamped onto
	// every row DecomposeUsageMarginal produces. PeriodEnd is the inclusive
	// last calendar day (matching RevenuePeriod), not the half-open exclusive
	// bound the curve input uses — add one day when building the curve input.
	PeriodStart time.Time
	PeriodEnd   time.Time

	// Subscription-level commitment config, mirrored from
	// subscription.Subscription for IsMultiPeriodCommitment.
	CommitmentAmount   *decimal.Decimal
	CommitmentDuration *types.BillingPeriod
	OverageFactor      *decimal.Decimal
	BillingPeriod      types.BillingPeriod
}

func (li PreviewLineItem) priceID() *string {
	if li.Price == nil || li.Price.ID == "" {
		return nil
	}
	return lo.ToPtr(li.Price.ID)
}

func (li PreviewLineItem) meterID() *string {
	if li.Meter == nil || li.Meter.ID == "" {
		return nil
	}
	return lo.ToPtr(li.Meter.ID)
}

func (li PreviewLineItem) aggregationType() *types.AggregationType {
	if li.Meter == nil {
		return nil
	}
	return lo.ToPtr(li.Meter.Aggregation.Type)
}

func (li PreviewLineItem) subLineItemID() *string {
	if li.SubLineItemID == "" {
		return nil
	}
	return lo.ToPtr(li.SubLineItemID)
}

// isCommitmentTrueupOrOverage reports whether li is the engine's synthetic
// true-up/overage line item (metadata-flagged, PriceType FIXED) rather than a
// regular fixed charge — DecomposeFixed excludes these.
func (li PreviewLineItem) isCommitmentTrueupOrOverage() bool {
	return li.Metadata.GetBool(types.MetadataKeyIsCommitmentTrueup) || li.Metadata.GetBool(types.MetadataKeyIsOverage)
}

// DayCharge is one calendar day's CUMULATIVE-through-that-day revenue
// decomposition for a single usage line item, produced by the curve service.
// Every field is a running total through Day, not a per-day marginal value —
// DecomposeUsageMarginal derives marginal(D) = cur.X - prev.X per field.
type DayCharge struct {
	Day time.Time

	// CumulativeCharge is CalculateCost(price, CumulativeBillableQty) — the
	// actual engine charge through this day, net of the entitlement limit.
	CumulativeCharge decimal.Decimal
	// CumulativeGrossQty is the raw cumulative metered quantity through this
	// day, before the entitlement limit is applied.
	CumulativeGrossQty decimal.Decimal
	// CumulativeBillableQty is max(0, CumulativeGrossQty - EntitlementLimit).
	CumulativeBillableQty decimal.Decimal
	// CumulativeEntitlementQty is min(CumulativeGrossQty, EntitlementLimit) —
	// the entitlement consumed earliest-first, purely as a function of gross usage.
	CumulativeEntitlementQty decimal.Decimal
	// UsageAtListRate is CumulativeGrossQty x tier1_rate: gross usage priced at
	// the list rate, before any entitlement giveback.
	UsageAtListRate decimal.Decimal
	// TierDelta is CumulativeCharge - (CumulativeBillableQty x tier1_rate) —
	// the deviation from a flat list-rate charge caused by graduated tiering
	// on the billed (post-entitlement) quantity. Zero for flat pricing, since
	// tier1_rate is then the only rate.
	TierDelta decimal.Decimal
}

// ClassifyDecompositionMode classifies a (price, meter) pair: PeriodOnly where
// a daily marginal split would misrepresent the charge (volume tiering
// re-rates every unit on the final tier reached; LATEST/AVG/WEIGHTED_SUM
// aggregations are not additive across days; a week/month-bucketed MAX
// resets on a boundary coarser than a day). Marginal otherwise.
func ClassifyDecompositionMode(p *price.Price, m *meter.Meter) types.DecompositionMode {
	if p != nil && p.TierMode == types.BILLING_TIER_VOLUME {
		return types.PeriodOnly
	}
	if m != nil {
		switch m.Aggregation.Type {
		case types.AggregationLatest, types.AggregationAvg, types.AggregationWeightedSum:
			return types.PeriodOnly
		}
	}
	if price.IsBucketedMax(p, m) {
		switch price.ResolveBucketSize(p, m) {
		case types.WindowSizeWeek, types.WindowSizeMonth:
			return types.PeriodOnly
		}
	}
	return types.Marginal
}

// IsMultiPeriodCommitment mirrors the billing engine's cumulative commitment
// detection — a subscription-level commitment whose duration spans more than
// one billing period (e.g. an ANNUAL commitment on a MONTHLY subscription).
// The rollup skips such subscriptions: their true-up can't be attributed to a
// single period.
func IsMultiPeriodCommitment(li PreviewLineItem) bool {
	if li.CommitmentAmount == nil || !li.CommitmentAmount.GreaterThan(decimal.Zero) {
		return false
	}
	if li.OverageFactor == nil || !li.OverageFactor.GreaterThan(decimal.NewFromInt(1)) {
		return false
	}
	if li.CommitmentDuration == nil || *li.CommitmentDuration == li.BillingPeriod {
		return false
	}
	return true
}

// cadenceDay returns the day a period_only row is stamped against: ADVANCE
// cadence bills before service delivery (period start), ARREAR after
// (period end).
func cadenceDay(cadence types.InvoiceCadence, period RevenuePeriod) time.Time {
	if cadence == types.InvoiceCadenceAdvance {
		return period.Start
	}
	return period.End
}

// invoiceCadence reads li.Price.InvoiceCadence, defaulting to ARREAR when no
// price is attached.
func invoiceCadence(li PreviewLineItem) types.InvoiceCadence {
	if li.Price == nil {
		return types.InvoiceCadenceArrear
	}
	return li.Price.InvoiceCadence
}

// newPeriodOnlyFact builds the fields common to every period_only
// revenue_facts row (fixed, usage period_only, commitment true-up).
func newPeriodOnlyFact(li PreviewLineItem, period RevenuePeriod, day time.Time, source types.RevenueSource) *RevenueFact {
	return &RevenueFact{
		ID:                types.GenerateUUIDWithPrefix("revfact"),
		TenantID:          li.TenantID,
		EnvironmentID:     li.EnvironmentID,
		CustomerID:        li.CustomerID,
		SubscriptionID:    li.SubscriptionID,
		SubLineItemID:     li.subLineItemID(),
		PriceID:           li.priceID(),
		RevenueSource:     source,
		PeriodStart:       period.Start,
		PeriodEnd:         period.End,
		Day:               day,
		NetAmount:         li.EngineAmount,
		DecompositionMode: types.PeriodOnly,
		Currency:          li.Currency,
		Status:            types.FactProvisional,
		Version:           1,
		ComputedAt:        time.Now().UTC(),
	}
}

// DecomposeFixed produces one period_only row for a FIXED-price line item,
// dated on its invoice cadence day. Returns nil for the engine's synthetic
// commitment true-up/overage line items (also PriceType FIXED) — those belong
// to DecomposeCommitmentTrueup instead.
func DecomposeFixed(li PreviewLineItem, period RevenuePeriod) *RevenueFact {
	if li.isCommitmentTrueupOrOverage() {
		return nil
	}
	return newPeriodOnlyFact(li, period, cadenceDay(invoiceCadence(li), period), types.RevenueSourceFixed)
}

// DecomposeUsagePeriodOnly produces one row for a usage line item whose
// (price, meter) pair classified PeriodOnly: the engine's period charge,
// un-split, dated on the same cadence rule as a fixed charge.
func DecomposeUsagePeriodOnly(li PreviewLineItem, period RevenuePeriod) *RevenueFact {
	f := newPeriodOnlyFact(li, period, cadenceDay(invoiceCadence(li), period), types.RevenueSourceUsage)
	f.MeterID = li.meterID()
	f.AggregationType = li.aggregationType()
	return f
}

// DecomposeCommitmentTrueup produces one row for the commitment true-up/
// overage charge, always dated at period end — the true-up amount is only
// known once the period's usage is final.
func DecomposeCommitmentTrueup(li PreviewLineItem, period RevenuePeriod) *RevenueFact {
	return newPeriodOnlyFact(li, period, period.End, types.RevenueSourceCommitmentTrueup)
}

// DecomposeUsageMarginal produces one row per DayCharge in curve. Each day's
// NetAmount/UsageAtListRate/TierDelta/EntitlementAmount/BillableQty/
// EntitlementQty is the marginal (day-over-day) delta of the cumulative
// curve, with curve[-1] treated as the zero DayCharge. By construction this
// reconciles exactly every day:
//
//	NetAmount == UsageAtListRate + TierDelta - EntitlementAmount
func DecomposeUsageMarginal(li PreviewLineItem, curve []DayCharge) []*RevenueFact {
	if len(curve) == 0 {
		return nil
	}

	tier1Rate := ListRate(li.Price)
	rows := make([]*RevenueFact, 0, len(curve))

	var prev DayCharge
	for _, dc := range curve {
		marginalBillableQty := dc.CumulativeBillableQty.Sub(prev.CumulativeBillableQty)
		marginalEntitlementQty := dc.CumulativeEntitlementQty.Sub(prev.CumulativeEntitlementQty)
		marginalCharge := dc.CumulativeCharge.Sub(prev.CumulativeCharge)
		marginalUsageAtListRate := dc.UsageAtListRate.Sub(prev.UsageAtListRate)
		marginalTierDelta := dc.TierDelta.Sub(prev.TierDelta)
		entitlementAmount := marginalEntitlementQty.Mul(tier1Rate)

		rows = append(rows, &RevenueFact{
			ID:                types.GenerateUUIDWithPrefix("revfact"),
			TenantID:          li.TenantID,
			EnvironmentID:     li.EnvironmentID,
			CustomerID:        li.CustomerID,
			SubscriptionID:    li.SubscriptionID,
			SubLineItemID:     li.subLineItemID(),
			PriceID:           li.priceID(),
			MeterID:           li.meterID(),
			AggregationType:   li.aggregationType(),
			RevenueSource:     types.RevenueSourceUsage,
			PeriodStart:       li.PeriodStart,
			PeriodEnd:         li.PeriodEnd,
			Day:               dc.Day,
			UsageAtListRate:   marginalUsageAtListRate,
			TierDelta:         marginalTierDelta,
			EntitlementAmount: entitlementAmount,
			NetAmount:         marginalCharge,
			BillableQty:       marginalBillableQty,
			EntitlementQty:    marginalEntitlementQty,
			DecompositionMode: types.Marginal,
			Currency:          li.Currency,
			Status:            types.FactProvisional,
			Version:           1,
			ComputedAt:        time.Now().UTC(),
		})
		prev = dc
	}

	return rows
}

// ListRate is the tier1/list rate used for UsageAtListRate and TierDelta: the
// first tier's unit amount for tiered pricing, or the flat unit amount
// otherwise (a flat price has one rate, which doubles as its own tier1).
func ListRate(p *price.Price) decimal.Decimal {
	if len(p.Tiers) > 0 {
		return p.Tiers[0].UnitAmount
	}
	return p.Amount
}
