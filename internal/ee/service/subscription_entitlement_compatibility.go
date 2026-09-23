package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/addonassociation"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// A metered feature measures usage over one reset period, so every config feeding it has to
// agree on which. validateEntitlementCompatibility checks the set the change LEAVES BEHIND
// rather than the one it starts from: a swap replacing A with B on a feature is legal even
// when their periods differ, while two adds that disagree with each other are not.
//
// Known limitation: a removal is resolved to its addon's entitlements, so removing one of
// several live instances of the same addon frees the feature here while another instance is
// still live. Same identity-versus-instance shape as ERD §3.1, and the same fix.
func (s *subscriptionGrantService) validateEntitlementCompatibility(
	ctx context.Context,
	req GrantChangeRequest,
) error {
	incomingByAddon, err := s.meteredEntitlementsByAddon(ctx, addonIDsOf(req.Incoming))
	if err != nil {
		return err
	}
	if len(incomingByAddon) == 0 {
		return nil
	}

	resetByFeature, err := s.survivingMeteredResetPeriods(ctx, req)
	if err != nil {
		return err
	}

	// Folded one at a time against the accumulating map, so two incoming configs that
	// disagree with each other are caught as well as one disagreeing with a survivor.
	for _, addonID := range addonIDsOf(req.Incoming) {
		for _, ent := range incomingByAddon[addonID] {
			existing, seen := resetByFeature[ent.FeatureID]
			if seen && existing != ent.UsageResetPeriod {
				return ierr.NewError("metered feature usage reset period conflict").
					WithHintf("Feature %s is already measured over %s, but this addon measures it over %s",
						ent.FeatureID, existing, ent.UsageResetPeriod).
					WithReportableDetails(map[string]any{
						"subscription_id": req.Sub.ID,
						"addon_id":        addonID,
						"feature_id":      ent.FeatureID,
					}).
					Mark(ierr.ErrValidation)
			}
			resetByFeature[ent.FeatureID] = ent.UsageResetPeriod
		}
	}

	return nil
}

// survivingMeteredResetPeriods is what still measures each metered feature once the change's
// removals are gone: the subscription's own configs minus the departing addons', plus the
// addons already attached but awaiting payment.
func (s *subscriptionGrantService) survivingMeteredResetPeriods(
	ctx context.Context,
	req GrantChangeRequest,
) (map[string]types.EntitlementUsageResetPeriod, error) {
	subSvc := &subscriptionService{ServiceParams: s.ServiceParams}

	current, err := subSvc.GetSubscriptionEntitlementsForSubscription(ctx, req.Sub)
	if err != nil {
		return nil, err
	}

	leaving := lo.SliceToMap(addonIDsOf(req.Removed), func(id string) (string, bool) { return id, true })

	resetByFeature := make(map[string]types.EntitlementUsageResetPeriod)
	for _, ent := range current {
		if ent == nil || ent.Entitlement == nil || ent.FeatureType != types.FeatureTypeMetered {
			continue
		}
		if ent.EntityType == types.ENTITLEMENT_ENTITY_TYPE_ADDON && leaving[ent.EntityID] {
			continue
		}
		resetByFeature[ent.FeatureID] = ent.UsageResetPeriod
	}

	// A pending attach holds its feature too, or a second attach could contradict it while
	// the first is still waiting to be paid. The addons this change is itself adding are
	// skipped: their own configs are folded in by the caller.
	arriving := lo.SliceToMap(addonIDsOf(req.Incoming), func(id string) (string, bool) { return id, true })
	pending, err := subSvc.listPendingAddonAssociations(ctx, req.Sub.ID)
	if err != nil {
		return nil, err
	}

	pendingIDs := lo.FilterMap(pending, func(a *addonassociation.AddonAssociation, _ int) (string, bool) {
		return a.AddonID, a != nil && !arriving[a.AddonID]
	})
	pendingByAddon, err := s.meteredEntitlementsByAddon(ctx, lo.Uniq(pendingIDs))
	if err != nil {
		return nil, err
	}
	for _, ents := range pendingByAddon {
		for _, ent := range ents {
			if _, seen := resetByFeature[ent.FeatureID]; !seen {
				resetByFeature[ent.FeatureID] = ent.UsageResetPeriod
			}
		}
	}

	return resetByFeature, nil
}

// meteredEntitlementsByAddon reads each addon's metered entitlements once.
func (s *subscriptionGrantService) meteredEntitlementsByAddon(
	ctx context.Context,
	addonIDs []string,
) (map[string][]*dto.EntitlementResponse, error) {
	entSvc := NewEntitlementService(s.ServiceParams)
	byAddon := make(map[string][]*dto.EntitlementResponse, len(addonIDs))

	for _, addonID := range lo.Uniq(addonIDs) {
		if _, read := byAddon[addonID]; read {
			continue
		}

		ents, err := entSvc.GetAddonEntitlements(ctx, addonID)
		if err != nil {
			return nil, err
		}

		metered := lo.Filter(ents.Items, func(e *dto.EntitlementResponse, _ int) bool {
			return e != nil && e.Entitlement != nil && e.FeatureType == types.FeatureTypeMetered
		})
		if len(metered) > 0 {
			byAddon[addonID] = metered
		}
	}

	return byAddon, nil
}

func addonIDsOf(sources []GrantSource) []string {
	return lo.Uniq(lo.Compact(lo.Map(sources, func(src GrantSource, _ int) string { return src.AddonID })))
}
