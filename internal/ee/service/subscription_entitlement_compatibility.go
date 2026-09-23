package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/addonassociation"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// featureHolder is one config measuring a feature, and when the change takes it away.
type featureHolder struct {
	reset types.EntitlementUsageResetPeriod
	// departsAt is set only for a config this change removes; until then it still measures.
	departsAt *time.Time
}

// validateEntitlementCompatibility rejects a change whose entries disagree about a feature,
// on either axis a shared feature has: how it is measured, and when the change reaches it.
// Both are judged on the set the change LEAVES BEHIND rather than the one it starts from, so
// a swap replacing A with B is legal where two adds contradicting each other are not.
//
// Known limitation: a removal is resolved to its addon's entitlements, so removing one of
// several live instances of the same addon frees the feature here while another instance is
// still live. Same identity-versus-instance shape as ERD §3.1, and the same fix.
func (s *subscriptionGrantService) validateEntitlementCompatibility(
	ctx context.Context,
	req GrantChangeRequest,
) error {
	incomingByAddon, err := s.entitlementsByAddon(ctx, addonIDsOf(req.Incoming))
	if err != nil {
		return err
	}
	if len(incomingByAddon) == 0 {
		return nil
	}

	if err := s.validateResetPeriods(ctx, req, incomingByAddon); err != nil {
		return err
	}

	return validateGrantDates(req, incomingByAddon)
}

// A metered feature measures usage over one reset period, so every config feeding it has to
// agree on which.
func (s *subscriptionGrantService) validateResetPeriods(
	ctx context.Context,
	req GrantChangeRequest,
	incomingByAddon map[string][]*dto.EntitlementResponse,
) error {
	holders, err := s.meteredFeatureHolders(ctx, req)
	if err != nil {
		return err
	}

	// Folded one entry at a time into the accumulating map, so two incoming configs that
	// disagree with each other are caught as well as one disagreeing with a survivor.
	for _, src := range req.Incoming {
		for _, ent := range incomingByAddon[src.AddonID] {
			if ent.FeatureType != types.FeatureTypeMetered {
				continue
			}

			for _, held := range holders[ent.FeatureID] {
				if held.reset == ent.UsageResetPeriod {
					continue
				}
				// A config already gone by the time this entry lands cannot conflict with it.
				if held.departsAt != nil && !held.departsAt.After(src.EffectiveDate) {
					continue
				}

				return ierr.NewError("metered feature usage reset period conflict").
					WithHintf("Feature %s is already measured over %s, but this addon measures it over %s",
						ent.FeatureID, held.reset, ent.UsageResetPeriod).
					WithReportableDetails(map[string]any{
						"subscription_id": req.Sub.ID,
						"addon_id":        src.AddonID,
						"feature_id":      ent.FeatureID,
					}).
					Mark(ierr.ErrValidation)
			}

			holders[ent.FeatureID] = append(holders[ent.FeatureID],
				featureHolder{reset: ent.UsageResetPeriod})
		}
	}

	return nil
}

// Addons funding one feature pool into a single grant window, and one window carries one
// coefficient, so entries landing on one feature must have asked for the same date. They are
// compared on the date requested rather than the date resolved: the resolved one is clamped
// per price, so two entries that asked to start together can resolve moments apart.
func validateGrantDates(
	req GrantChangeRequest,
	incomingByAddon map[string][]*dto.EntitlementResponse,
) error {
	dateByFeature := make(map[string]time.Time)
	addonByFeature := make(map[string]string)

	for _, src := range req.Incoming {
		// Mirrors sourceGrantECs: an entry that opens no window this cycle cannot collide.
		if src.ChangeType == types.ScheduleTypePeriodEnd ||
			!src.requestedDate().Before(req.Sub.CurrentPeriodEnd) {
			continue
		}

		for _, ent := range incomingByAddon[src.AddonID] {
			if ent.Entitlement == nil || !ent.Entitlement.HasGrantConfig() {
				continue
			}

			at, seen := dateByFeature[ent.FeatureID]
			if !seen {
				dateByFeature[ent.FeatureID] = src.requestedDate()
				addonByFeature[ent.FeatureID] = src.AddonID
				continue
			}
			if at.Equal(src.requestedDate()) {
				continue
			}

			return ierr.NewError("addons granting the same feature must share an effective date").
				WithHint("Send them as separate changes, or give both the same date").
				WithReportableDetails(map[string]any{
					"subscription_id": req.Sub.ID,
					"feature_id":      ent.FeatureID,
					"addon_ids":       []string{addonByFeature[ent.FeatureID], src.AddonID},
					"dates":           []string{at.String(), src.requestedDate().String()},
				}).
				Mark(ierr.ErrValidation)
		}
	}

	return nil
}

// meteredFeatureHolders is everything measuring each metered feature once the change lands:
// the subscription's own configs, the addons already attached but awaiting payment, and the
// departing configs with the date they leave — a period-end removal still measures its
// feature until then.
func (s *subscriptionGrantService) meteredFeatureHolders(
	ctx context.Context,
	req GrantChangeRequest,
) (map[string][]featureHolder, error) {
	subSvc := &subscriptionService{ServiceParams: s.ServiceParams}

	current, err := subSvc.GetSubscriptionEntitlementsForSubscription(ctx, req.Sub)
	if err != nil {
		return nil, err
	}

	departures := make(map[string]time.Time, len(req.Removed))
	for _, src := range req.Removed {
		if src.AddonID == "" {
			continue
		}
		if at, seen := departures[src.AddonID]; !seen || src.EffectiveDate.Before(at) {
			departures[src.AddonID] = src.EffectiveDate
		}
	}

	holders := make(map[string][]featureHolder)
	for _, ent := range current {
		if ent == nil || ent.Entitlement == nil || ent.FeatureType != types.FeatureTypeMetered {
			continue
		}

		held := featureHolder{reset: ent.UsageResetPeriod}
		if ent.EntityType == types.ENTITLEMENT_ENTITY_TYPE_ADDON {
			if at, leaving := departures[ent.EntityID]; leaving {
				held.departsAt = lo.ToPtr(at)
			}
		}
		holders[ent.FeatureID] = append(holders[ent.FeatureID], held)
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
	pendingByAddon, err := s.entitlementsByAddon(ctx, lo.Uniq(pendingIDs))
	if err != nil {
		return nil, err
	}
	for _, ents := range pendingByAddon {
		for _, ent := range ents {
			if ent.FeatureType != types.FeatureTypeMetered {
				continue
			}
			holders[ent.FeatureID] = append(holders[ent.FeatureID],
				featureHolder{reset: ent.UsageResetPeriod})
		}
	}

	return holders, nil
}

// entitlementsByAddon reads each addon once and keeps what either check needs: metered
// entitlements for the reset period, grant-bearing ones for the pooling date.
func (s *subscriptionGrantService) entitlementsByAddon(
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

		relevant := lo.Filter(ents.Items, func(e *dto.EntitlementResponse, _ int) bool {
			return e != nil && e.Entitlement != nil &&
				(e.FeatureType == types.FeatureTypeMetered || e.Entitlement.HasGrantConfig())
		})
		if len(relevant) > 0 {
			byAddon[addonID] = relevant
		}
	}

	return byAddon, nil
}

func addonIDsOf(sources []GrantSource) []string {
	return lo.Uniq(lo.Compact(lo.Map(sources, func(src GrantSource, _ int) string { return src.AddonID })))
}
