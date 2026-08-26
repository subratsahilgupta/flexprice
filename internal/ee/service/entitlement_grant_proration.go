package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entitlement"
	"github.com/flexprice/flexprice/internal/domain/entitlementgrant"
	"github.com/flexprice/flexprice/internal/domain/proration"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// entitlementGrantQuotaScale matches the numeric(25,15) precision of entitlement_grants.quota.
const entitlementGrantQuotaScale = 15

func (s *subscriptionService) resolveGrantProration(
	ctx context.Context,
	sub *subscription.Subscription,
	incomingECs []*entitlement.Entitlement,
	existingByFeature map[string][]*entitlement.Entitlement,
	effectiveDate time.Time,
	behavior types.ProrationBehavior,
	source string,
) ([]*entitlementgrant.EntitlementGrant, error) {
	if s.EntitlementGrantRepo == nil || s.EntitlementRepo == nil {
		return nil, nil
	}

	incoming := lo.Filter(incomingECs, func(ec *entitlement.Entitlement, _ int) bool {
		return ec != nil && ec.HasGrantConfig()
	})
	if len(incoming) == 0 {
		return nil, nil
	}

	p, err := types.FindPeriodForDate(&types.FindPeriodForDateParams{
		Target:           effectiveDate,
		KnownPeriodStart: sub.CurrentPeriodStart,
		KnownPeriodEnd:   sub.CurrentPeriodEnd,
		Anchor:           sub.BillingAnchor,
		PeriodCount:      sub.BillingPeriodCount,
		BillingPeriod:    sub.BillingPeriod,
		Timezone:         sub.Timezone,
	})
	if err != nil {
		s.Logger.Info(ctx, "skipping entitlement grant proration; could not resolve billing period",
			"subscription_id", sub.ID,
			"effective_date", effectiveDate,
			"error", err.Error())
		return nil, nil
	}

	prorationDate := p.Start
	if behavior == types.ProrationBehaviorCreateProrations && effectiveDate.After(p.Start) {
		prorationDate = effectiveDate
	}

	coefficient, err := proration.Coefficient(p.Start, p.End, prorationDate, types.StrategySecondBased)
	if err != nil {
		s.Logger.Info(ctx, "skipping entitlement grant proration; coefficient could not be computed",
			"subscription_id", sub.ID,
			"effective_date", effectiveDate,
			"error", err.Error())
		return nil, nil
	}

	grants := make([]*entitlementgrant.EntitlementGrant, 0, len(incoming))
	for featureID, featureECs := range lo.GroupBy(incoming, func(ec *entitlement.Entitlement) string {
		return ec.FeatureID
	}) {
		originalQuota := decimal.Zero
		for _, ec := range featureECs {
			originalQuota = originalQuota.Add(lo.FromPtr(ec.GrantQuota))
		}

		delta := originalQuota.Mul(coefficient).Round(entitlementGrantQuotaScale)
		if !delta.IsPositive() {
			s.Logger.Info(ctx, "skipping entitlement grant proration; quota is not positive",
				"subscription_id", sub.ID,
				"feature_id", featureID,
				"coefficient", coefficient.String())
			continue
		}

		// A feature the subscription already had allowance for keeps the cycle's window; one
		// arriving now starts at the change, so it meters only the time it was paid for.
		// A predecessor overrides this when the grant is materialized.
		coverageStart := p.Start
		if len(existingByFeature[featureID]) == 0 {
			coverageStart = effectiveDate
		}

		grants = append(grants, entitlementgrant.NewEntitlementGrantBuilder(nil).
			WithCustomerID(sub.CustomerID).
			WithSubscriptionID(sub.ID).
			WithScope(types.EntitlementGrantScopeFeature, featureID).
			WithMeasure(featureECs[0].GrantMeasure).
			WithQuota(delta).
			WithWindow(coverageStart, p.End).
			WithMetadata(proration.AuditMetadata(proration.AuditParams{
				Source:        source,
				Coefficient:   coefficient,
				OriginalKey:   "proration_original_quota",
				OriginalValue: originalQuota,
				PeriodStart:   p.Start,
				PeriodEnd:     p.End,
				ProrationDate: prorationDate,
				Strategy:      types.StrategySecondBased,
			})).
			Build())
	}

	return grants, nil
}

// materialiseEntitlementGrants writes the resolved grants for the cycle: it closes every
// live window of the features the change touches, then builds the open requests for the
// ones this path owns.
func (s *subscriptionService) materialiseEntitlementGrants(
	ctx context.Context,
	sub *subscription.Subscription,
	newGrants []*entitlementgrant.EntitlementGrant,
	incomingECs []*entitlement.Entitlement,
	existingByFeature map[string][]*entitlement.Entitlement,
	effectiveDate time.Time,
) error {
	if len(newGrants) == 0 {
		return nil
	}

	liveByFeature, err := s.liveGrantsByFeature(ctx, sub, effectiveDate)
	if err != nil {
		return err
	}

	// Every live row of a feature the change touches, whatever its aggregation mode: an
	// additive feature contributes its single pooled row, a parallel one contributes each
	// EC's slot. Features with no new grant keep their windows untouched.
	toClose := lo.FlatMap(newGrants, func(g *entitlementgrant.EntitlementGrant, _ int) []*entitlementgrant.EntitlementGrant {
		return liveByFeature[g.FeatureID()]
	})
	if len(toClose) == 0 {
		return nil
	}

	grantSvc := NewEntitlementGrantService(s.ServiceParams)

	closedByID, err := grantSvc.CloseEntitlementGrants(ctx, toClose)
	if err != nil {
		return err
	}

	incomingByFeature := lo.GroupBy(
		lo.Filter(incomingECs, func(ec *entitlement.Entitlement, _ int) bool {
			return ec != nil && ec.HasGrantConfig()
		}),
		func(ec *entitlement.Entitlement) string { return ec.FeatureID },
	)

	reqs := make([]OpenFeatureBasedEntitlementGrantsRequest, 0, len(newGrants))
	for _, g := range newGrants {
		featureID := g.FeatureID()
		incoming := incomingByFeature[featureID]
		existing := existingByFeature[featureID]

		if !shouldOpenGrantManually(append(append([]*entitlement.Entitlement{}, existing...), incoming...)) {
			continue
		}

		req := OpenFeatureBasedEntitlementGrantsRequest{
			FeatureID:   featureID,
			New:         g,
			ExistingECs: existing,
			IncomingECs: incoming,
		}

		if live := lo.FirstOrEmpty(liveByFeature[featureID]); live != nil {
			closedGrant := closedByID[live.ID]

			req.Closed = closedGrant
			req.New = entitlementgrant.NewEntitlementGrantBuilder(g).
				WithWindow(closedGrant.ValidTo, g.ValidTo).
				Build()
		}

		reqs = append(reqs, req)
	}
	if len(reqs) == 0 {
		return nil
	}

	_, err = grantSvc.OpenFeatureBasedEntitlementGrants(ctx, reqs)
	return err
}

func shouldOpenGrantManually(featureECs []*entitlement.Entitlement) bool {
	if len(featureECs) == 0 {
		return false
	}

	return lo.EveryBy(featureECs, func(ec *entitlement.Entitlement) bool {
		return ec != nil &&
			defaultedMode(ec.AggregationMode) == types.EntitlementAggregationModeAdditive &&
			ec.GrantDurationUnit == types.EntitlementGrantDurationUnitSubscriptionPeriod
	})
}

// -----------------------------------------------------------------------------
// read helpers
// -----------------------------------------------------------------------------

// GetSubscriptionGrantECsByFeature is the subscription's grant ECs grouped by feature —
// the set that decides slot ownership and the cold-start quota. Called before the incoming
// ECs are persisted, so they are absent from the result.
func (s *subscriptionService) GetSubscriptionGrantECsByFeature(
	ctx context.Context,
	sub *subscription.Subscription,
) (map[string][]*entitlement.Entitlement, error) {
	ents, err := s.GetSubscriptionEntitlementsForSubscription(ctx, sub)
	if err != nil {
		return nil, err
	}

	grantECs := lo.FilterMap(ents, func(e *dto.EntitlementResponse, _ int) (*entitlement.Entitlement, bool) {
		if e == nil || e.Entitlement == nil || !e.Entitlement.HasGrantConfig() {
			return nil, false
		}
		return e.Entitlement, true
	})

	return lo.GroupBy(grantECs, func(ec *entitlement.Entitlement) string {
		return ec.FeatureID
	}), nil
}

// liveGrantsByFeature returns the subscription's feature-scoped grant rows whose window
// contains `at`, grouped by feature. Windows already closed before `at` are excluded by the
// query, so each slot yields the one segment that is actually live.
func (s *subscriptionService) liveGrantsByFeature(
	ctx context.Context,
	sub *subscription.Subscription,
	at time.Time,
) (map[string][]*entitlementgrant.EntitlementGrant, error) {
	filter := types.NewNoLimitEntitlementGrantFilter().
		WithCustomerIDs(sub.CustomerID).
		WithSubscriptionIDs(sub.ID).
		WithLiveOnly(at)

	rows, err := s.EntitlementGrantRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	byFeature := make(map[string][]*entitlementgrant.EntitlementGrant)
	for _, g := range rows {
		if g == nil || !g.IsFeatureScoped() {
			continue
		}
		byFeature[g.FeatureID()] = append(byFeature[g.FeatureID()], g)
	}
	return byFeature, nil
}
