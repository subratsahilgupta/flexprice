package service

// Grant hooks for the entitlement write path: what an entitlement create, update or
// delete does to the windows already materialized for it. The grant service owns the
// windows themselves; this file is only the entitlement side of the handover.

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entitlement"
	"github.com/flexprice/flexprice/internal/domain/meter"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// deriveGrantConfig puts every new metered entitlement on the grant model so the legacy
// set stops growing. usage_limit becomes the quota, its absence an unlimited allowance,
// and the billing period the cadence — which reproduces legacy behaviour exactly.
// Meters a grant cannot cover keep the legacy shape rather than lose their entitlement.
func (s *entitlementService) deriveGrantConfig(
	ctx context.Context,
	e *entitlement.Entitlement,
	m *meter.Meter,
) error {
	if e == nil || e.FeatureType != types.FeatureTypeMetered || e.HasGrantConfig() {
		return nil
	}

	measure := types.EntitlementGrantMeasureQuantity
	if err := s.grantMeterEligibility(ctx, m, measure); err != nil {
		s.Logger.Info(ctx, "feature cannot carry an allowance; entitlement stays on the legacy model",
			"feature_id", e.FeatureID,
			"meter_id", lo.FromPtr(m).ID,
			"reason", err.Error())
		return nil
	}

	e.GrantMeasure = measure
	e.GrantDurationUnit = types.EntitlementGrantDurationUnitSubscriptionPeriod
	if e.AggregationMode == "" {
		e.AggregationMode = types.EntitlementAggregationModeAdditive
	}
	if e.UsageLimit != nil {
		e.GrantQuota = lo.ToPtr(decimal.NewFromInt(*e.UsageLimit))
	}

	e.UsageLimit = nil

	s.Logger.Info(ctx, "derived a grant config for a new metered entitlement",
		"feature_id", e.FeatureID,
		"unlimited", e.IsUnlimitedGrant())
	return nil
}

// grantConfigMoved reports whether an update changed the allowance itself, as opposed
// to restating it or touching an unrelated field.
func grantConfigMoved(before, after *entitlement.Entitlement) bool {
	return before.GrantMeasure != after.GrantMeasure ||
		before.GrantDurationUnit != after.GrantDurationUnit ||
		before.GrantAllocationBehavior != after.GrantAllocationBehavior ||
		before.AggregationMode != after.AggregationMode ||
		lo.FromPtr(before.GrantDurationValue) != lo.FromPtr(after.GrantDurationValue) ||
		!quotaUnchanged(before.GrantQuota, after.GrantQuota)
}

// resettleGrantWindows re-cuts one customer's live windows after an allowance edit.
// Subscription rows only: a plan edit would rewrite every subscriber's window at once.
func (s *entitlementService) resettleGrantWindows(
	ctx context.Context,
	e *entitlement.Entitlement,
	priorQuota *decimal.Decimal,
	priorUnlimited bool,
) error {
	if e == nil || e.EntityType != types.ENTITLEMENT_ENTITY_TYPE_SUBSCRIPTION || !e.HasGrantConfig() {
		return nil
	}
	unlimited := e.IsUnlimitedGrant()
	if unlimited == priorUnlimited && quotaUnchanged(priorQuota, e.GrantQuota) {
		return nil
	}

	// The allowance replaced the old one rather than topping it up, so the delta is the
	// difference. Added to what the closed window had left, that is the same number as
	// recalculating from scratch: (old − usage) + (new − old) = new − usage.
	delta := lo.FromPtr(e.GrantQuota).Sub(lo.FromPtr(priorQuota))
	return s.reissueGrantWindows(ctx, e, delta, unlimited, "entitlement_updated")
}

// reissueGrantWindows hands the change to the grant service, which reads the windows and
// the configs funding them as they stand when it runs.
func (s *entitlementService) reissueGrantWindows(
	ctx context.Context,
	e *entitlement.Entitlement,
	delta decimal.Decimal,
	unlimited bool,
	source string,
) error {
	_, err := NewEntitlementGrantService(s.ServiceParams).ReissueEntitlementGrants(ctx, &dto.ReissueEntitlementGrantsRequest{
		SubscriptionID: e.EntityID,
		FeatureID:      e.FeatureID,
		Delta:          delta,
		Unlimited:      unlimited,
		At:             time.Now().UTC(),
		Source:         source,
	})
	return err
}

// assertSingleContributor refuses an override where several entitlements already feed the
// feature. One allowance field cannot address more than one of them: additive pools them,
// so the customer lands above the number typed, and parallel leaves it ambiguous which
// was meant. Lifting this needs per-allowance editing, not a change here.
func (s *entitlementService) assertSingleContributor(ctx context.Context, e *entitlement.Entitlement) error {
	if !isSubscriptionOverride(e) {
		return nil
	}
	parentID := lo.FromPtr(e.ParentEntitlementID)

	sub, err := s.SubRepo.Get(ctx, e.EntityID)
	if err != nil {
		return err
	}

	entitlementsForSubscription, err := NewSubscriptionService(s.ServiceParams).GetSubscriptionEntitlementsForSubscription(ctx, sub)

	if err != nil {
		return err
	}

	otherEntitlements := make([]string, 0, len(entitlementsForSubscription))
	for _, other := range entitlementsForSubscription {
		if other == nil || other.Entitlement == nil || other.FeatureID != e.FeatureID {
			continue
		}
		// The row being written and the one it replaces both become this override.
		if other.ID == e.ID || other.ID == parentID {
			continue
		}
		otherEntitlements = append(otherEntitlements, other.ID)
	}

	if len(otherEntitlements) == 0 {
		return nil
	}

	return ierr.NewError("this allowance comes from more than one entitlement").
		WithHint("Change it on the plan or addon instead.").
		WithReportableDetails(map[string]interface{}{
			"subscription_id":       e.EntityID,
			"feature_id":            e.FeatureID,
			"other_entitlement_ids": otherEntitlements,
		}).
		Mark(ierr.ErrValidation)
}

// isSubscriptionOverride: replaces another entitlement for one subscription. A plan or
// addon row is a rule for everyone on it; a net-new subscription row replaces nothing.
func isSubscriptionOverride(e *entitlement.Entitlement) bool {
	return e != nil &&
		e.EntityType == types.ENTITLEMENT_ENTITY_TYPE_SUBSCRIPTION &&
		lo.FromPtr(e.ParentEntitlementID) != ""
}

// takeOverGrantWindowsFromParent runs on a first override: the override replaces the
// plan's rule, so the live window is re-cut by the difference between the two.
func (s *entitlementService) takeOverGrantWindowsFromParent(ctx context.Context, e *entitlement.Entitlement) error {
	parent, err := s.EntitlementRepo.Get(ctx, lo.FromPtr(e.ParentEntitlementID))
	if err != nil {
		return err
	}
	delta := lo.FromPtr(e.GrantQuota).Sub(lo.FromPtr(parent.GrantQuota))
	return s.reissueGrantWindows(ctx, e, delta, e.IsUnlimitedGrant(), "override_created")
}

// settleGrantWindowsForDeletedEC hands an override's allowance back to its parent;
// anything else pooled, so the pool re-cuts without it — the addon detach path.
func (s *entitlementService) settleGrantWindowsForDeletedEC(ctx context.Context, e *entitlement.Entitlement) error {
	if e == nil || e.EntityType != types.ENTITLEMENT_ENTITY_TYPE_SUBSCRIPTION || !e.HasGrantConfig() {
		return nil
	}
	if s.EntitlementGrantRepo == nil {
		return nil
	}

	if parentID := lo.FromPtr(e.ParentEntitlementID); parentID != "" {
		parent, err := s.EntitlementRepo.Get(ctx, parentID)
		if err != nil {
			return err
		}
		// A legacy parent has no allowance to hand the window to; it runs out instead.
		if !parent.HasGrantConfig() {
			return nil
		}
		delta := lo.FromPtr(parent.GrantQuota).Sub(lo.FromPtr(e.GrantQuota))
		return s.reissueGrantWindows(ctx, e, delta, parent.IsUnlimitedGrant(), "override_removed")
	}

	sub, err := s.SubRepo.Get(ctx, e.EntityID)
	if err != nil {
		return err
	}
	// The addon paths reach this through Resolve, which builds the config from addon
	// sources. A deleted entitlement has no addon behind it, so the config is built
	// here — one entitlement leaving, nothing arriving.
	grantSvc := newSubscriptionGrantService(s.ServiceParams)
	removed := []*entitlement.Entitlement{e}
	surviving, err := grantSvc.resolveSurvivingGrantECs(ctx, sub, removed)
	if err != nil {
		return err
	}

	return grantSvc.applyEntitlementGrantChange(ctx, &GrantChangeConfig{
		sub:                     sub,
		entitlementsToRemove:    removed,
		survivingECsByFeature:   surviving,
		entitlementChangeAt:     time.Now().UTC(),
		entitlementChangeOrigin: grantProrationSourceEntitlementGone,
	})
}

func quotaUnchanged(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
