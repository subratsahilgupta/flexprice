package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/revenuefact"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/utils"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// Skip reasons logged by RollupSubscription. Discounted invoices and
// multi-period commitments are skipped whole rather than written wrong.
const (
	revenueRollupSkipDiscountUnsupported   = "discount_unsupported"
	revenueRollupSkipMultiPeriodCommitment = "multi_period_commitment"
	revenueRollupSkipOverageUnsupported    = "overage_unsupported"
)

// RevenueService writes and maintains revenue_facts rows. It is a shadow
// write-path: it re-runs the billing preview, never mutates billing state,
// and logs reconciliation mismatches instead of blocking on them.
type RevenueService interface {
	// RollupSubscription splits the subscription's current billing period
	// into PROVISIONAL revenue_facts rows. Subscriptions it cannot split
	// faithfully (discounts, multi-period commitments) are skipped with a log.
	RollupSubscription(ctx context.Context, subscriptionID string) error

	// RollupDirty rolls every opted-in subscription with activity since the
	// given time. Over-rolling is harmless: the upsert is idempotent.
	RollupDirty(ctx context.Context, since time.Time) (rolled, skipped int, err error)

	// FinalizeSubscriptionPeriod flips the invoice's PROVISIONAL rows to
	// FINAL and stamps them with the invoice. When no provisional rows exist
	// it derives rows from the invoice itself first. Runs async after invoice
	// finalization.
	FinalizeSubscriptionPeriod(ctx context.Context, invoiceID string) error

	// RevertInvoiceFacts writes a negating twin for every FINAL row of a
	// voided invoice — FINAL rows are never edited. Idempotent; runs async
	// after VoidInvoice.
	RevertInvoiceFacts(ctx context.Context, invoiceID string) error
}

type revenueService struct {
	ServiceParams
}

// NewRevenueService returns the revenue_facts service.
func NewRevenueService(params ServiceParams) RevenueService {
	return &revenueService{ServiceParams: params}
}

// lineItemRows keeps one line item's rows together for reconciliation logging.
type lineItemRows struct {
	rows       []*revenuefact.RevenueFact
	amount     decimal.Decimal
	identifier string
}

func (s *revenueService) RollupSubscription(ctx context.Context, subscriptionID string) error {
	_, err := s.rollupSubscription(ctx, subscriptionID)
	return err
}

// rollupSubscription also reports policy skips, which RollupDirty tallies but
// the interface method does not expose.
func (s *revenueService) rollupSubscription(ctx context.Context, subscriptionID string) (skipped bool, err error) {
	sub, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		return false, err
	}
	return s.rollupSubscriptionForPeriod(ctx, sub, sub.CurrentPeriodStart, sub.CurrentPeriodEnd)
}

// rollupSubscriptionForPeriod rolls one explicit billing window (periodEnd
// exclusive). The schedule targets the current period; backfills and the
// finalize fallback target past ones.
func (s *revenueService) rollupSubscriptionForPeriod(ctx context.Context, sub *subscription.Subscription, periodStart, periodEnd time.Time) (skipped bool, err error) {
	subscriptionID := sub.ID

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

	// The preview does not resolve coupon amounts onto line items, so treat any
	// coupon reference or already-set discount as "discounted" and skip — a
	// conservative superset that never lets a discounted invoice through
	// mis-split.
	if hasDiscount(invReq) {
		s.Logger.Info(ctx, "revenue_rollup_skipped",
			"reason", revenueRollupSkipDiscountUnsupported,
			"subscription_id", subscriptionID)
		return true, nil
	}

	// CommitmentAmount/CommitmentDuration/OverageFactor/BillingPeriod are
	// subscription-wide, so one probe line item covers every real line item.
	probe := previewLineItem{
		CommitmentAmount:   sub.CommitmentAmount,
		CommitmentDuration: sub.CommitmentDuration,
		OverageFactor:      sub.OverageFactor,
		BillingPeriod:      sub.BillingPeriod,
	}
	if isMultiPeriodCommitment(probe) {
		s.Logger.Info(ctx, "revenue_rollup_skipped",
			"reason", revenueRollupSkipMultiPeriodCommitment,
			"subscription_id", subscriptionID)
		return true, nil
	}

	// When usage exceeds a commitment the engine reduces the usage line and
	// adds an is_overage line; splitting the reduced line per day would not
	// reconcile, so skip the whole subscription.
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
		// internal type here (revenuePeriod, previewLineItem.PeriodEnd) is inclusive
		// of the last calendar day, so convert once at the boundary.
		itemPeriod := revenuePeriod{Start: itemPeriodStart, End: itemPeriodEndExclusive.AddDate(0, 0, -1)}

		itemSubscriptionID := sub.ID
		if item.SubscriptionID != nil && *item.SubscriptionID != "" {
			itemSubscriptionID = *item.SubscriptionID
		}
		itemCustomerID := sub.CustomerID
		if childID, ok := item.Metadata[string(types.InvoiceLineItemMetadataKeyChildCustomerID)]; ok && childID != "" {
			itemCustomerID = childID
		}

		base := previewLineItem{
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
		if base.SubLineItemID == "" {
			// Aggregate lines (subscription-level true-up) carry no line-item
			// id; fall back to the subscription id so the provisional grain
			// stays non-null.
			base.SubLineItemID = itemSubscriptionID
		}

		isTrueup := item.Metadata.GetBool(types.MetadataKeyIsCommitmentTrueup)
		isOverage := item.Metadata.GetBool(types.MetadataKeyIsOverage)

		switch {
		case isTrueup || isOverage:
			// The engine assigns these lines a fresh random price_id every
			// compute; use a stable synthetic id so recomputes update in place.
			base.Price = &price.Price{ID: stableTrueupPriceID(base.SubLineItemID, itemSubscriptionID, isOverage)}

			row := decomposeCommitmentTrueup(base, itemPeriod)
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
			if row := decomposeFixed(base, itemPeriod); row != nil {
				allRows = append(allRows, row)
				groups = append(groups, lineItemRows{rows: []*revenuefact.RevenueFact{row}, amount: item.Amount, identifier: lo.FromPtr(item.PriceID)})
			}

		case lo.FromPtr(item.PriceType) == string(types.PRICE_TYPE_USAGE):
			// A usage line item whose price or meter fails to
			// hydrate is a hard error — never let decompositionMode(nil, nil)
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
			switch decompositionMode(p, m) {
			case types.Marginal:
				entitlementLimit, allowErr := resolveEntitlementLimit(m.ID)
				if allowErr != nil {
					return false, allowErr
				}
				curve, curveErr := s.buildUsageCurve(ctx, usageCurveInput{
					Price:       p,
					MeterID:     m.ID,
					PeriodStart: itemPeriod.Start,
					// BuildUsageCurve's upper bound is exclusive — convert back from the inclusive day.
					PeriodEnd:        itemPeriod.exclusiveEnd(),
					EntitlementLimit: entitlementLimit,
					Timezone:         sub.Timezone,
				})
				if curveErr != nil {
					return false, curveErr
				}
				rows = decomposeUsageMarginal(base, curve)
			default:
				rows = []*revenuefact.RevenueFact{decomposeUsagePeriodOnly(base, itemPeriod)}
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

	// Every row is PROVISIONAL with a non-empty price_id by construction;
	// re-running upserts in place instead of duplicating.
	if err := s.RevenueFactRepo.UpsertProvisional(ctx, allRows); err != nil {
		return false, err
	}

	return false, nil
}

func (s *revenueService) RollupDirty(ctx context.Context, since time.Time) (rolled, skipped int, err error) {
	// Never scan all subscriptions: only tenants opted in via
	// revenue_analytics_config are considered, one (tenant, environment) at a
	// time so the per-env listing uses the subscriptions index.
	tenantEnvConfigs, err := s.SettingsRepo.ListAllTenantEnvSettingsByKey(ctx, types.SettingKeyRevenueAnalyticsConfig)
	if err != nil {
		return 0, 0, err
	}

	for _, tec := range tenantEnvConfigs {
		cfg, cfgErr := utils.ToStruct[types.RevenueAnalyticsConfig](tec.Config)
		if cfgErr != nil {
			s.Logger.Info(ctx, "skipping tenant with malformed revenue_analytics_config",
				"tenant_id", tec.TenantID,
				"environment_id", tec.EnvironmentID,
				"error", cfgErr)
			continue
		}
		if !cfg.Enabled {
			continue
		}

		tenantCtx := types.SetTenantID(ctx, tec.TenantID)
		tenantCtx = types.SetEnvironmentID(tenantCtx, tec.EnvironmentID)

		envRolled, envSkipped, envErr := s.rollupDirtyForEnvironment(tenantCtx, since)
		if envErr != nil {
			// One environment's listing failure must not abort the whole scan.
			s.Logger.Error(ctx, "revenue rollup dirty scan failed for environment",
				"error", envErr,
				"tenant_id", tec.TenantID,
				"environment_id", tec.EnvironmentID)
			continue
		}
		rolled += envRolled
		skipped += envSkipped
	}

	return rolled, skipped, nil
}

// rollupDirtyForEnvironment scans one (tenant, environment)'s active
// subscriptions in pages and rolls every one with activity since `since`.
func (s *revenueService) rollupDirtyForEnvironment(ctx context.Context, since time.Time) (rolled, skipped int, err error) {
	const batchSize = 1000
	offset := 0

	for {
		filter := types.NewSubscriptionFilter()
		filter.Limit = lo.ToPtr(batchSize)
		filter.Offset = lo.ToPtr(offset)
		filter.Status = lo.ToPtr(types.StatusPublished)
		filter.SubscriptionStatus = []types.SubscriptionStatus{types.SubscriptionStatusActive}
		subs, listErr := s.SubRepo.List(ctx, filter)
		if listErr != nil {
			return rolled, skipped, listErr
		}
		if len(subs) == 0 {
			return rolled, skipped, nil
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

		if len(subs) < batchSize {
			return rolled, skipped, nil
		}
		offset += batchSize
	}
}

// finalizePeriodGroup identifies one (subscription, period) grain touched by
// FinalizeSubscriptionPeriod, used to re-list the FINAL rows for
// reconciliation after every line item's flip has been issued.
type finalizePeriodGroup struct {
	subscriptionID string
	periodStart    time.Time
	periodEnd      time.Time
}

func (s *revenueService) FinalizeSubscriptionPeriod(ctx context.Context, invoiceID string) error {
	inv, err := s.InvoiceRepo.Get(ctx, invoiceID)
	if err != nil {
		return err
	}

	flipped, groupSeen, err := s.flipInvoiceLineItems(ctx, inv)
	if err != nil {
		return err
	}
	if len(groupSeen) == 0 {
		return nil
	}

	// Zero flipped rows means no provisional rows existed for this period
	// (re-drafted after a void, or never scheduled). Derive rows from the
	// finalized invoice itself, then flip again.
	if flipped == 0 {
		if jitErr := s.rollupFromInvoice(ctx, inv); jitErr != nil {
			return jitErr
		}
		if flipped, _, err = s.flipInvoiceLineItems(ctx, inv); err != nil {
			return err
		}
		if flipped == 0 {
			// Still nothing to stamp (e.g. only non-subscription line items) —
			// surface the gap, never block.
			s.Logger.Info(ctx, "revenue_facts_flip_gap", "invoice_id", invoiceID)
		}
	}

	// Re-check that the flipped rows sum to the invoice total. A mismatch is
	// logged, never blocks — and rows from other invoices in the same window
	// are excluded by the invoice_id filter.
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

// flipInvoiceLineItems issues one FlipToFinal per flippable line item of inv,
// returning the total rows flipped and the distinct (subscription, period)
// grains touched.
func (s *revenueService) flipInvoiceLineItems(ctx context.Context, inv *invoice.Invoice) (int, map[string]finalizePeriodGroup, error) {
	flipped := 0
	groupSeen := make(map[string]finalizePeriodGroup)

	for _, li := range inv.LineItems {
		// Join on the LINE ITEM's own subscription, never the invoice's —
		// grouped invoicing can bill several child subscriptions on one
		// invoice, each carrying its own revenue_facts rows.
		subscriptionID := lo.FromPtr(li.SubscriptionID)
		periodStart := lo.FromPtr(li.PeriodStart)
		periodEndExclusive := lo.FromPtr(li.PeriodEnd)
		if subscriptionID == "" || periodStart.IsZero() || periodEndExclusive.IsZero() {
			// Non-subscription line items (one-off charges, credits, etc.) never
			// produced revenue_facts rows in RollupSubscription — nothing to flip.
			continue
		}
		// The invoice line item's period end is half-open/exclusive, same as
		// the preview engine's — convert to the inclusive day bound
		// revenue_facts rows are keyed/queried on.
		periodEnd := periodEndExclusive.AddDate(0, 0, -1)

		priceID := lo.FromPtr(li.PriceID)
		isTrueup := li.Metadata.GetBool(types.MetadataKeyIsCommitmentTrueup)
		isOverage := li.Metadata.GetBool(types.MetadataKeyIsOverage)
		if isTrueup || isOverage {
			// True-up/overage lines carry a random price_id per compute; match
			// the provisional rows by re-deriving the same stable synthetic id.
			priceID = stableTrueupPriceID(lo.FromPtr(li.SubscriptionLineItemID), subscriptionID, isOverage)
		}

		n, err := s.RevenueFactRepo.FlipToFinal(ctx, subscriptionID, priceID, periodStart, periodEnd, inv.ID, li.ID)
		if err != nil {
			return flipped, groupSeen, err
		}
		flipped += n

		key := fmt.Sprintf("%s|%d|%d", subscriptionID, periodStart.UnixNano(), periodEnd.UnixNano())
		groupSeen[key] = finalizePeriodGroup{subscriptionID: subscriptionID, periodStart: periodStart, periodEnd: periodEnd}
	}

	return flipped, groupSeen, nil
}

// rollupFromInvoice derives period_only PROVISIONAL rows from a finalized
// invoice.s own line items — the fallback when none exist for its period.
// Per-day usage splitting is deliberately not reconstructed here.
func (s *revenueService) rollupFromInvoice(ctx context.Context, inv *invoice.Invoice) error {
	priceCache := map[string]*price.Price{}
	meterCache := map[string]*meter.Meter{}
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	var rows []*revenuefact.RevenueFact
	for _, li := range inv.LineItems {
		subscriptionID := lo.FromPtr(li.SubscriptionID)
		periodStart := lo.FromPtr(li.PeriodStart)
		periodEndExclusive := lo.FromPtr(li.PeriodEnd)
		if subscriptionID == "" || periodStart.IsZero() || periodEndExclusive.IsZero() {
			continue
		}
		period := revenuePeriod{Start: periodStart, End: periodEndExclusive.AddDate(0, 0, -1)}

		base := previewLineItem{
			TenantID:       tenantID,
			EnvironmentID:  environmentID,
			CustomerID:     li.CustomerID,
			SubscriptionID: subscriptionID,
			SubLineItemID:  lo.FromPtr(li.SubscriptionLineItemID),
			Currency:       li.Currency,
			Metadata:       li.Metadata,
			EngineAmount:   li.Amount,
			PeriodStart:    period.Start,
			PeriodEnd:      period.End,
		}
		if base.SubLineItemID == "" {
			// Invoice line items may lack a subscription-line-item id; the
			// invoice line item id is stable for this invoice.
			base.SubLineItemID = li.ID
		}

		isTrueup := li.Metadata.GetBool(types.MetadataKeyIsCommitmentTrueup)
		isOverage := li.Metadata.GetBool(types.MetadataKeyIsOverage)
		switch {
		case isTrueup || isOverage:
			base.Price = &price.Price{ID: stableTrueupPriceID(base.SubLineItemID, subscriptionID, isOverage)}
			rows = append(rows, decomposeCommitmentTrueup(base, period))

		case lo.FromPtr(li.PriceType) == string(types.PRICE_TYPE_FIXED):
			p, err := s.getPrice(ctx, priceCache, lo.FromPtr(li.PriceID))
			if err != nil {
				return err
			}
			base.Price = p
			if row := decomposeFixed(base, period); row != nil {
				rows = append(rows, row)
			}

		case lo.FromPtr(li.PriceType) == string(types.PRICE_TYPE_USAGE):
			p, err := s.getPrice(ctx, priceCache, lo.FromPtr(li.PriceID))
			if err != nil {
				return err
			}
			base.Price = p
			if meterID := lo.FromPtr(li.MeterID); meterID != "" {
				m, err := s.getMeter(ctx, meterCache, meterID)
				if err != nil {
					return err
				}
				base.Meter = m
			}
			rows = append(rows, decomposeUsagePeriodOnly(base, period))

		default:
			s.Logger.Info(ctx, "revenue_jit_rollup_line_item_skipped",
				"invoice_id", inv.ID,
				"invoice_line_item_id", li.ID,
				"price_type", lo.FromPtr(li.PriceType))
		}
	}

	if len(rows) == 0 {
		return nil
	}
	return s.RevenueFactRepo.UpsertProvisional(ctx, rows)
}

// RevertInvoiceFacts posts contra rows for every FINAL fact stamped with a
// now-voided invoice — FINAL rows are immutable, so a void reverses them
// rather than editing or deleting. Idempotent via the repository.
func (s *revenueService) RevertInvoiceFacts(ctx context.Context, invoiceID string) error {
	n, err := s.RevenueFactRepo.RevertByInvoice(ctx, invoiceID)
	if err != nil {
		return err
	}
	if n > 0 {
		s.Logger.Info(ctx, "revenue facts reverted for voided invoice",
			"invoice_id", invoiceID, "revert_rows", n)
	}
	return nil
}

// stableTrueupPriceID is the synthetic price_id for true-up/overage rows —
// stable across recomputes, unlike the engine.s per-compute random id. Falls
// back to the subscription id when there is no line-item id.
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

func (s *revenueService) getPrice(ctx context.Context, cache map[string]*price.Price, id string) (*price.Price, error) {
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

func (s *revenueService) getMeter(ctx context.Context, cache map[string]*meter.Meter, id string) (*meter.Meter, error) {
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

// loadEntitlementsByMeterID returns each metered entitlement.s usage limit,
// keyed by meter id — the free-quantity input buildUsageCurve needs.
func (s *revenueService) loadEntitlementsByMeterID(ctx context.Context, subscriptionID string) (map[string]decimal.Decimal, error) {
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

// invoiceDiscountTotal sums line and invoice discounts. Zero today: a
// non-zero discount already makes hasDiscount skip the subscription.
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

// hasDiscount reports whether the preview carries any discount signal:
// coupon references, or an already-set discount amount.
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
