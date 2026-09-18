package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/revenuefact"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// Skip reasons logged/metriced by RollupSubscription. Slice-1 scope excludes
// discounted invoices (decomposition doesn't split discounts yet) and
// multi-period commitments (the true-up can't be attributed to one period).
const (
	revenueRollupSkipDiscountUnsupported   = "discount_unsupported"
	revenueRollupSkipMultiPeriodCommitment = "multi_period_commitment"
	revenueRollupSkipOverageUnsupported    = "overage_unsupported"
)

// RevenueRollupService is the linchpin of the revenue_facts shadow write-path:
// preview -> curve -> decompose -> reconcile -> upsert. It never mutates
// billing state — PrepareSubscriptionInvoiceRequest is non-mutating, and
// reconciliation mismatches are only logged, never blocking the write.
type RevenueRollupService interface {
	// RollupSubscription previews subscription's current open period, decomposes
	// every line item into revenue_facts rows, and upserts them as PROVISIONAL.
	// Discounted and multi-period-commitment subscriptions are skipped (logged,
	// nil error) rather than partially/incorrectly decomposed.
	RollupSubscription(ctx context.Context, subscriptionID string) error

	// RollupDirty rolls every subscription with activity since `since` (a coarse
	// updated_at/period-bounds scan — idempotent upsert makes over-rolling
	// harmless) and tallies how many were rolled vs skipped.
	RollupDirty(ctx context.Context, since time.Time) (rolled, skipped int, err error)

	// FinalizeSubscriptionPeriod flips the PROVISIONAL revenue_facts rows backing
	// invoiceID's line items to FINAL, stamping invoice_id/invoice_line_item_id,
	// then re-asserts reconciliation over the now-FINAL rows (shadow-only: a
	// mismatch is logged, the flip is never rolled back). Intended to run
	// asynchronously after invoice finalization — see performFinalizeInvoiceActions.
	FinalizeSubscriptionPeriod(ctx context.Context, invoiceID string) error
}

type revenueRollupService struct {
	ServiceParams
}

// NewRevenueRollupService constructs the rollup service from the shared
// service dependencies (repos, logger) — no dependencies beyond ServiceParams.
func NewRevenueRollupService(params ServiceParams) RevenueRollupService {
	return &revenueRollupService{ServiceParams: params}
}

// lineItemRows is one preview line item's decomposed rows, kept together for
// reconcileLineItem and for reconciliation logging.
type lineItemRows struct {
	rows       []*revenuefact.RevenueFact
	amount     decimal.Decimal
	identifier string
}

func (s *revenueRollupService) RollupSubscription(ctx context.Context, subscriptionID string) error {
	_, err := s.rollupSubscription(ctx, subscriptionID)
	return err
}

// rollupSubscription is the real implementation; it additionally reports
// whether the subscription was skipped by policy (discount/multi-period),
// which RollupDirty needs for its rolled/skipped tally but the public
// RollupSubscription signature (fixed by the interface) cannot carry.
func (s *revenueRollupService) rollupSubscription(ctx context.Context, subscriptionID string) (skipped bool, err error) {
	sub, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		return false, err
	}

	periodStart := sub.CurrentPeriodStart
	periodEnd := sub.CurrentPeriodEnd

	billingSvc := NewBillingService(s.ServiceParams)
	invReq, err := billingSvc.PrepareSubscriptionInvoiceRequest(ctx, &dto.PrepareSubscriptionInvoiceRequestParams{
		Subscription:   sub,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		ReferencePoint: types.ReferencePointPreview,
	})
	if err != nil {
		return false, err
	}

	if len(invReq.LineItems) == 0 {
		return false, nil
	}

	// PrepareSubscriptionInvoiceRequest does not resolve coupon discount
	// amounts onto the DTO line items (that happens later, in the invoice-assembly/
	// coupon-application path this rollup never invokes) — so the earliest reliable
	// discount signal here is the coupon reference lists it does populate, plus any
	// discount amount that IS already set. Treating either as "carries a discount"
	// is a conservative superset: it may skip a sub whose coupon nets to $0, but it
	// never lets a genuinely discounted invoice through undecomposed.
	if hasDiscount(invReq) {
		s.Logger.Info(ctx, "revenue_rollup_skipped",
			"reason", revenueRollupSkipDiscountUnsupported,
			"subscription_id", subscriptionID)
		return true, nil
	}

	// CommitmentAmount/CommitmentDuration/OverageFactor/BillingPeriod are
	// subscription-wide, so one probe line item covers every real line item.
	probe := revenuefact.PreviewLineItem{
		CommitmentAmount:   sub.CommitmentAmount,
		CommitmentDuration: sub.CommitmentDuration,
		OverageFactor:      sub.OverageFactor,
		BillingPeriod:      sub.BillingPeriod,
	}
	if revenuefact.IsMultiPeriodCommitment(probe) {
		s.Logger.Info(ctx, "revenue_rollup_skipped",
			"reason", revenueRollupSkipMultiPeriodCommitment,
			"subscription_id", subscriptionID)
		return true, nil
	}

	// A single-period commitment that usage EXCEEDS makes the
	// engine emit a REDUCED within-commitment usage line plus a separate
	// is_overage line. Decomposing the reduced line via the full curve (no
	// commitment knowledge) overstates usage and won't reconcile — skip the
	// whole subscription rather than write wrong rows. Commitment-aware usage
	// decomposition is a deliberate later slice.
	if hasOverageLine(invReq) {
		s.Logger.Info(ctx, "revenue_rollup_skipped",
			"reason", revenueRollupSkipOverageUnsupported,
			"subscription_id", subscriptionID)
		return true, nil
	}

	priceCache := map[string]*price.Price{}
	meterCache := map[string]*meter.Meter{}
	entitlementLimitCache := map[string]decimal.Decimal{}
	var entitlementLimitErr error
	var entitlementLimitLoaded bool

	resolveEntitlementLimit := func(meterID string) (decimal.Decimal, error) {
		if !entitlementLimitLoaded {
			entitlementLimitLoaded = true
			entitlementLimitCache, entitlementLimitErr = s.loadEntitlementsByMeterID(ctx, subscriptionID)
		}
		if entitlementLimitErr != nil {
			return decimal.Zero, entitlementLimitErr
		}
		return entitlementLimitCache[meterID], nil
	}

	var allRows []*revenuefact.RevenueFact
	var groups []lineItemRows

	for i := range invReq.LineItems {
		item := &invReq.LineItems[i]

		itemPeriodStart := lo.FromPtr(item.PeriodStart)
		if itemPeriodStart.IsZero() {
			itemPeriodStart = periodStart
		}
		itemPeriodEndExclusive := lo.FromPtr(item.PeriodEnd)
		if itemPeriodEndExclusive.IsZero() {
			itemPeriodEndExclusive = periodEnd
		}
		// The engine's line item period end is half-open/exclusive; every
		// internal type here (RevenuePeriod, PreviewLineItem.PeriodEnd) is inclusive
		// of the last calendar day, so convert once at the boundary.
		itemPeriod := revenuefact.RevenuePeriod{Start: itemPeriodStart, End: itemPeriodEndExclusive.AddDate(0, 0, -1)}

		itemSubscriptionID := sub.ID
		if item.SubscriptionID != nil && *item.SubscriptionID != "" {
			itemSubscriptionID = *item.SubscriptionID
		}
		itemCustomerID := sub.CustomerID
		if childID, ok := item.Metadata[string(types.InvoiceLineItemMetadataKeyChildCustomerID)]; ok && childID != "" {
			itemCustomerID = childID
		}

		base := revenuefact.PreviewLineItem{
			TenantID:       sub.TenantID,
			EnvironmentID:  sub.EnvironmentID,
			CustomerID:     itemCustomerID,
			SubscriptionID: itemSubscriptionID,
			SubLineItemID:  lo.FromPtr(item.SubscriptionLineItemID),
			Currency:       sub.Currency,
			Metadata:       item.Metadata,
			EngineAmount:   item.Amount,
			PeriodStart:    itemPeriod.Start,
			PeriodEnd:      itemPeriod.End,
		}

		isTrueup := item.Metadata.GetBool(types.MetadataKeyIsCommitmentTrueup)
		isOverage := item.Metadata.GetBool(types.MetadataKeyIsOverage)

		switch {
		case isTrueup || isOverage:
			// The engine emits a fresh random price_id for these rows
			// on every compute; forwarding it verbatim would make every recompute
			// INSERT a new provisional row instead of updating one. Derive a stable
			// synthetic id instead — see stableTrueupPriceID.
			base.Price = &price.Price{ID: stableTrueupPriceID(base.SubLineItemID, itemSubscriptionID, isOverage)}

			row := revenuefact.DecomposeCommitmentTrueup(base, itemPeriod)
			allRows = append(allRows, row)
			groups = append(groups, lineItemRows{rows: []*revenuefact.RevenueFact{row}, amount: item.Amount, identifier: base.Price.ID})

		case lo.FromPtr(item.PriceType) == string(types.PRICE_TYPE_FIXED):
			p, hydrateErr := s.getPrice(ctx, priceCache, lo.FromPtr(item.PriceID))
			if hydrateErr != nil {
				return false, ierr.WithError(hydrateErr).
					WithHint("failed to hydrate price for fixed line item").
					Mark(ierr.ErrSystem)
			}
			base.Price = p
			if row := revenuefact.DecomposeFixed(base, itemPeriod); row != nil {
				allRows = append(allRows, row)
				groups = append(groups, lineItemRows{rows: []*revenuefact.RevenueFact{row}, amount: item.Amount, identifier: lo.FromPtr(item.PriceID)})
			}

		case lo.FromPtr(item.PriceType) == string(types.PRICE_TYPE_USAGE):
			// A usage line item whose price or meter fails to
			// hydrate is a hard error — never let revenuefact.ClassifyDecompositionMode(nil, nil)
			// silently default to Marginal.
			p, hydrateErr := s.getPrice(ctx, priceCache, lo.FromPtr(item.PriceID))
			if hydrateErr != nil {
				return false, ierr.WithError(hydrateErr).
					WithHint("failed to hydrate price for usage line item").
					WithReportableDetails(map[string]any{"price_id": lo.FromPtr(item.PriceID), "subscription_id": subscriptionID}).
					Mark(ierr.ErrSystem)
			}
			m, hydrateErr := s.getMeter(ctx, meterCache, lo.FromPtr(item.MeterID))
			if hydrateErr != nil {
				return false, ierr.WithError(hydrateErr).
					WithHint("failed to hydrate meter for usage line item").
					WithReportableDetails(map[string]any{"meter_id": lo.FromPtr(item.MeterID), "subscription_id": subscriptionID}).
					Mark(ierr.ErrSystem)
			}
			base.Price = p
			base.Meter = m

			var rows []*revenuefact.RevenueFact
			switch revenuefact.ClassifyDecompositionMode(p, m) {
			case types.Marginal:
				entitlementLimit, allowErr := resolveEntitlementLimit(m.ID)
				if allowErr != nil {
					return false, allowErr
				}
				curve, curveErr := NewRevenueCurveService(s.ServiceParams).BuildUsageCurve(ctx, LineItemPricingInput{
					Price:       p,
					MeterID:     m.ID,
					PeriodStart: itemPeriod.Start,
					// BuildUsageCurve's upper bound is exclusive — convert back from the inclusive day.
					PeriodEnd:        itemPeriod.ExclusiveEnd(),
					EntitlementLimit: entitlementLimit,
					Timezone:         sub.Timezone,
				})
				if curveErr != nil {
					return false, curveErr
				}
				rows = revenuefact.DecomposeUsageMarginal(base, curve)
			default:
				rows = []*revenuefact.RevenueFact{revenuefact.DecomposeUsagePeriodOnly(base, itemPeriod)}
			}
			allRows = append(allRows, rows...)
			groups = append(groups, lineItemRows{rows: rows, amount: item.Amount, identifier: lo.FromPtr(item.PriceID)})

		default:
			s.Logger.Info(ctx, "revenue_rollup_line_item_skipped",
				"subscription_id", subscriptionID,
				"price_type", lo.FromPtr(item.PriceType),
				"sub_line_item_id", base.SubLineItemID)
		}
	}

	// Reconciliation is shadow-only. A mismatch is logged/metriced —
	// never blocks the write, and billing itself is never touched.
	for _, g := range groups {
		for _, row := range g.rows {
			if residual, ok := reconcileRow(row); !ok {
				s.Logger.Info(ctx, "revenue_reconciliation_mismatch",
					"scope", "row", "subscription_id", subscriptionID,
					"fact_id", row.ID, "day", row.Day, "residual", residual.String())
			}
		}
		if residual, ok := reconcileLineItem(g.rows, g.amount); !ok {
			s.Logger.Info(ctx, "revenue_reconciliation_mismatch",
				"scope", "line_item", "subscription_id", subscriptionID,
				"line_item_id", g.identifier, "residual", residual.String())
		}
	}
	// Subtract discounts so this stays correct if the
	// discount gate above is ever narrowed. A no-op today since hasDiscount
	// already skips any subscription carrying a discount signal.
	if residual, ok := reconcileInvoice(allRows, invReq.Subtotal.Sub(invoiceDiscountTotal(invReq))); !ok {
		s.Logger.Info(ctx, "revenue_reconciliation_mismatch",
			"scope", "invoice", "subscription_id", subscriptionID, "residual", residual.String())
	}

	if len(allRows) == 0 {
		return false, nil
	}

	// PROVISIONAL status and a non-empty price_id on every row are
	// enforced by the four decompose* constructors and the stable-id
	// substitution above; UpsertProvisional bumps version on a repeat call
	// against the same grain instead of inserting a duplicate.
	if err := s.RevenueFactRepo.UpsertProvisional(ctx, allRows); err != nil {
		return false, err
	}

	return false, nil
}

func (s *revenueRollupService) RollupDirty(ctx context.Context, since time.Time) (rolled, skipped int, err error) {
	filter := types.NewNoLimitSubscriptionFilter()
	filter.SubscriptionStatus = []types.SubscriptionStatus{types.SubscriptionStatusActive}
	subs, err := s.SubRepo.ListAll(ctx, filter)
	if err != nil {
		return 0, 0, err
	}

	for _, sub := range subs {
		if sub.UpdatedAt.Before(since) && sub.CurrentPeriodStart.Before(since) && sub.CurrentPeriodEnd.Before(since) {
			continue
		}

		wasSkipped, rollErr := s.rollupSubscription(ctx, sub.ID)
		if rollErr != nil {
			// Shadow write-path: one subscription's failure must never abort the
			// batch. Logged loud (never silent), tallied as skipped since it did
			// not produce rows.
			s.Logger.Error(ctx, "revenue rollup failed for subscription",
				"error", rollErr, "subscription_id", sub.ID)
			skipped++
			continue
		}
		if wasSkipped {
			skipped++
			continue
		}
		rolled++
	}

	return rolled, skipped, nil
}

// finalizePeriodGroup identifies one (subscription, period) grain touched by
// FinalizeSubscriptionPeriod, used to re-list the FINAL rows for
// reconciliation after every line item's flip has been issued.
type finalizePeriodGroup struct {
	subscriptionID string
	periodStart    time.Time
	periodEnd      time.Time
}

func (s *revenueRollupService) FinalizeSubscriptionPeriod(ctx context.Context, invoiceID string) error {
	inv, err := s.InvoiceRepo.Get(ctx, invoiceID)
	if err != nil {
		return err
	}

	groupSeen := make(map[string]finalizePeriodGroup)

	for _, li := range inv.LineItems {
		// Join on the LINE ITEM's own subscription, never the
		// invoice's — grouped invoicing can bill several child subscriptions on
		// one invoice, each carrying its own revenue_facts rows.
		subscriptionID := lo.FromPtr(li.SubscriptionID)
		periodStart := lo.FromPtr(li.PeriodStart)
		periodEndExclusive := lo.FromPtr(li.PeriodEnd)
		if subscriptionID == "" || periodStart.IsZero() || periodEndExclusive.IsZero() {
			// Non-subscription line items (one-off charges, credits, etc.) never
			// produced revenue_facts rows in RollupSubscription — nothing to flip.
			continue
		}
		// The invoice line item's period end is
		// half-open/exclusive, same as the preview engine's — convert to the
		// inclusive day bound revenue_facts rows are keyed/queried on.
		periodEnd := periodEndExclusive.AddDate(0, 0, -1)

		priceID := lo.FromPtr(li.PriceID)
		isTrueup := li.Metadata.GetBool(types.MetadataKeyIsCommitmentTrueup)
		isOverage := li.Metadata.GetBool(types.MetadataKeyIsOverage)
		if isTrueup || isOverage {
			// The finalized line item's own price_id is a fresh random one the
			// engine assigned at compute time (see stableTrueupPriceID) — it will
			// never match the provisional rows, which were written keyed on the
			// re-derived stable synthetic id. Re-derive the same id here.
			priceID = stableTrueupPriceID(lo.FromPtr(li.SubscriptionLineItemID), subscriptionID, isOverage)
		}

		if _, err := s.RevenueFactRepo.FlipToFinal(ctx, subscriptionID, priceID, periodStart, periodEnd, invoiceID, li.ID); err != nil {
			return err
		}

		key := fmt.Sprintf("%s|%d|%d", subscriptionID, periodStart.UnixNano(), periodEnd.UnixNano())
		groupSeen[key] = finalizePeriodGroup{subscriptionID: subscriptionID, periodStart: periodStart, periodEnd: periodEnd}
	}

	if len(groupSeen) == 0 {
		return nil
	}

	// Shadow-only: re-assert reconciliation over every row this
	// invoice actually flipped (rows from another invoice sharing the same
	// subscription/period window are excluded by the invoice_id filter below).
	// A mismatch is logged/metriced — never blocks or reverts the flip.
	var finalRows []*revenuefact.RevenueFact
	for _, g := range groupSeen {
		rows, err := s.RevenueFactRepo.ListBySubscriptionPeriod(ctx, g.subscriptionID, g.periodStart, g.periodEnd, types.FactFinal)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.InvoiceID != nil && *r.InvoiceID == invoiceID {
				finalRows = append(finalRows, r)
			}
		}
	}

	if residual, ok := reconcileInvoice(finalRows, inv.Subtotal.Sub(inv.TotalDiscount)); !ok {
		s.Logger.Info(ctx, "revenue_reconciliation_mismatch",
			"scope", "invoice_final", "invoice_id", invoiceID, "residual", residual.String())
	}

	return nil
}

// stableTrueupPriceID derives the stable synthetic price_id used for
// commitment-trueup/overage revenue_facts rows, standing in for the fresh
// random price_id the billing engine assigns those line items on every
// compute. Both the provisional rollup and the FINAL-flip call this
// so they always agree on which row to touch: the real per-line-item id when
// there is one, falling back to the subscription id for the
// subscription-aggregate true-up, which carries no sub_line_item_id at all.
func stableTrueupPriceID(subLineItemID, subscriptionID string, isOverage bool) string {
	stableID := subLineItemID
	if stableID == "" {
		stableID = subscriptionID
	}
	prefix := "trueup:"
	if isOverage {
		prefix = "overage:"
	}
	return prefix + stableID
}

func (s *revenueRollupService) getPrice(ctx context.Context, cache map[string]*price.Price, id string) (*price.Price, error) {
	if id == "" {
		return nil, ierr.NewError("price id is required").
			WithHint("cannot hydrate a price with an empty id").
			Mark(ierr.ErrValidation)
	}
	if p, ok := cache[id]; ok {
		return p, nil
	}
	p, err := s.PriceRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	cache[id] = p
	return p, nil
}

func (s *revenueRollupService) getMeter(ctx context.Context, cache map[string]*meter.Meter, id string) (*meter.Meter, error) {
	if id == "" {
		return nil, ierr.NewError("meter id is required").
			WithHint("cannot hydrate a meter with an empty id").
			Mark(ierr.ErrValidation)
	}
	if m, ok := cache[id]; ok {
		return m, nil
	}
	m, err := s.MeterRepo.GetMeter(ctx, id)
	if err != nil {
		return nil, err
	}
	cache[id] = m
	return m, nil
}

// loadEntitlementsByMeterID resolves every metered entitlement's usage limit
// for subscriptionID, keyed by meter id — the EntitlementLimit BuildUsageCurve
// needs to derive CumulativeEntitlementQty/EntitlementAmount, which the preview
// engine's own AdjustedEntitlementQuantity (already net of the limit) can't supply.
func (s *revenueRollupService) loadEntitlementsByMeterID(ctx context.Context, subscriptionID string) (map[string]decimal.Decimal, error) {
	subscriptionService := NewSubscriptionService(s.ServiceParams)
	agg, err := subscriptionService.GetAggregatedSubscriptionEntitlements(ctx, subscriptionID, nil)
	if err != nil {
		return nil, err
	}

	limits := make(map[string]decimal.Decimal)
	for _, f := range agg.Features {
		if f.Feature == nil || f.Entitlement == nil {
			continue
		}
		if types.FeatureType(f.Feature.Type) != types.FeatureTypeMetered || f.Feature.MeterID == "" {
			continue
		}
		if !f.Entitlement.IsEnabled || f.Entitlement.UsageLimit == nil {
			continue
		}
		limits[f.Feature.MeterID] = decimal.NewFromInt(*f.Entitlement.UsageLimit)
	}
	return limits, nil
}

// hasOverageLine reports whether any of invReq's line items is the engine's
// synthetic overage charge (metadata-flagged) — see the ruling at the Fix-1
// call site for why the whole subscription is skipped rather than decomposed.
func hasOverageLine(invReq *dto.CreateInvoiceRequest) bool {
	for _, li := range invReq.LineItems {
		if li.Metadata.GetBool(types.MetadataKeyIsOverage) {
			return true
		}
	}
	return false
}

// invoiceDiscountTotal sums the discount amount fields hasDiscount already
// treats as a (currently unpopulated at this stage) discount signal — see
// hasDiscount's own comment. Always zero today since a non-zero value here
// would already have tripped hasDiscount and skipped the subscription.
func invoiceDiscountTotal(invReq *dto.CreateInvoiceRequest) decimal.Decimal {
	total := decimal.Zero
	for _, li := range invReq.LineItems {
		if li.LineItemDiscount != nil {
			total = total.Add(*li.LineItemDiscount)
		}
		if li.InvoiceLevelDiscount != nil {
			total = total.Add(*li.InvoiceLevelDiscount)
		}
	}
	return total
}

// hasDiscount reports whether invReq's preview carries any discount signal —
// see the ruling-4 comment at its call site for why the coupon reference
// lists are the reliable signal here, not the (unpopulated at this stage)
// LineItemDiscount/InvoiceLevelDiscount amounts.
func hasDiscount(invReq *dto.CreateInvoiceRequest) bool {
	if len(invReq.InvoiceCoupons) > 0 || len(invReq.LineItemCoupons) > 0 {
		return true
	}
	for _, li := range invReq.LineItems {
		if li.LineItemDiscount != nil && !li.LineItemDiscount.IsZero() {
			return true
		}
		if li.InvoiceLevelDiscount != nil && !li.InvoiceLevelDiscount.IsZero() {
			return true
		}
	}
	return false
}
