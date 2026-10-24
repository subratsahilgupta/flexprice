package service

import (
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entitlement"
	"github.com/flexprice/flexprice/internal/domain/feature"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// An override past its end_date drops out and the plan entitlement it replaced returns.
func (s *SubscriptionServiceSuite) TestActiveEntitlements_HonourOverrideDates() {
	ctx := s.GetContext()
	subService := s.service.(*subscriptionService)
	sub := s.testData.subscription

	featureID := "feat_dated_override"
	s.NoError(s.GetStores().FeatureRepo.Create(ctx, &feature.Feature{
		ID:        featureID,
		Name:      "Rate limit config",
		Type:      types.FeatureTypeStatic,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}))

	planEnt := &entitlement.Entitlement{
		ID:          "ent_dated_plan",
		EntityType:  types.ENTITLEMENT_ENTITY_TYPE_PLAN,
		EntityID:    sub.PlanID,
		FeatureID:   featureID,
		FeatureType: types.FeatureTypeStatic,
		IsEnabled:   true,
		StaticValue: "100",
		BaseModel:   types.GetDefaultBaseModel(ctx),
	}
	_, err := s.GetStores().EntitlementRepo.Create(ctx, planEnt)
	s.Require().NoError(err)

	overrideStart := sub.CurrentPeriodStart.Add(24 * time.Hour)
	overrideEnd := sub.CurrentPeriodStart.Add(5 * 24 * time.Hour)
	_, err = s.GetStores().EntitlementRepo.Create(ctx, &entitlement.Entitlement{
		ID:                  "ent_dated_override",
		EntityType:          types.ENTITLEMENT_ENTITY_TYPE_SUBSCRIPTION,
		EntityID:            sub.ID,
		FeatureID:           featureID,
		FeatureType:         types.FeatureTypeStatic,
		IsEnabled:           true,
		StaticValue:         "500",
		ParentEntitlementID: lo.ToPtr(planEnt.ID),
		StartDate:           &overrideStart,
		EndDate:             &overrideEnd,
		BaseModel:           types.GetDefaultBaseModel(ctx),
	})
	s.Require().NoError(err)

	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"before start_date", sub.CurrentPeriodStart.Add(time.Hour), "100"},
		{"inside window", sub.CurrentPeriodStart.Add(2 * 24 * time.Hour), "500"},
		{"at end_date", overrideEnd, "100"},
		{"after end_date", overrideEnd.Add(24 * time.Hour), "100"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			ents, err := subService.GetActiveSubscriptionEntitlementsForSubscription(ctx, sub, tt.at)
			s.Require().NoError(err)
			got := lo.Filter(ents, func(e *dto.EntitlementResponse, _ int) bool { return e.FeatureID == featureID })
			s.Require().Len(got, 1)
			s.Equal(tt.want, got[0].StaticValue)
		})
	}
}
