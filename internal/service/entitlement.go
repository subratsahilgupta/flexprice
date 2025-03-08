package service

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entitlement"
	"github.com/flexprice/flexprice/internal/domain/feature"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/plan"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// EntitlementService defines the interface for entitlement operations
type EntitlementService interface {
	CreateEntitlement(ctx context.Context, req dto.CreateEntitlementRequest) (*dto.EntitlementResponse, error)
	GetEntitlement(ctx context.Context, id string) (*dto.EntitlementResponse, error)
	ListEntitlements(ctx context.Context, filter *types.EntitlementFilter) (*dto.ListEntitlementsResponse, error)
	UpdateEntitlement(ctx context.Context, id string, req dto.UpdateEntitlementRequest) (*dto.EntitlementResponse, error)
	DeleteEntitlement(ctx context.Context, id string) error
	GetPlanEntitlements(ctx context.Context, planID string) (*dto.ListEntitlementsResponse, error)
	GetPlanFeatureEntitlements(ctx context.Context, planID, featureID string) (*dto.ListEntitlementsResponse, error)
}

type entitlementService struct {
	repo        entitlement.Repository
	planRepo    plan.Repository
	featureRepo feature.Repository
	meterRepo   meter.Repository
	log         *logger.Logger
}

func NewEntitlementService(
	repo entitlement.Repository,
	planRepo plan.Repository,
	featureRepo feature.Repository,
	meterRepo meter.Repository,
	log *logger.Logger,
) EntitlementService {
	return &entitlementService{
		repo:        repo,
		planRepo:    planRepo,
		featureRepo: featureRepo,
		meterRepo:   meterRepo,
		log:         log,
	}
}

func (s *entitlementService) CreateEntitlement(ctx context.Context, req dto.CreateEntitlementRequest) (*dto.EntitlementResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if req.PlanID == "" {
		return nil, ierr.NewError("plan_id is required").
			Mark(ierr.ErrValidation)
	}

	// Validate plan exists
	plan, err := s.planRepo.Get(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}

	// Validate feature exists
	feature, err := s.featureRepo.Get(ctx, req.FeatureID)
	if err != nil {
		return nil, err
	}

	// Validate feature type matches
	if feature.Type != req.FeatureType {
		return nil, ierr.NewError("feature type mismatch").
			WithHint(fmt.Sprintf("Expected %s, got %s", feature.Type, req.FeatureType)).
			WithReportableDetails(map[string]interface{}{
				"expected": feature.Type,
				"actual":   req.FeatureType,
			}).
			Mark(ierr.ErrValidation)
	}

	// Create entitlement
	e := req.ToEntitlement(ctx)

	result, err := s.repo.Create(ctx, e)
	if err != nil {
		return nil, err
	}

	response := &dto.EntitlementResponse{Entitlement: result}

	// Add expanded fields
	response.Feature = &dto.FeatureResponse{Feature: feature}
	response.Plan = &dto.PlanResponse{Plan: plan}

	return response, nil
}

func (s *entitlementService) GetEntitlement(ctx context.Context, id string) (*dto.EntitlementResponse, error) {

	result, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	response := &dto.EntitlementResponse{Entitlement: result}

	// Add expanded fields
	feature, err := s.featureRepo.Get(ctx, result.FeatureID)
	if err != nil {
		return nil, err
	}
	response.Feature = &dto.FeatureResponse{Feature: feature}

	plan, err := s.planRepo.Get(ctx, result.PlanID)
	if err != nil {
		return nil, err
	}
	response.Plan = &dto.PlanResponse{Plan: plan}

	return response, nil
}

func (s *entitlementService) ListEntitlements(ctx context.Context, filter *types.EntitlementFilter) (*dto.ListEntitlementsResponse, error) {
	if filter == nil {
		filter = types.NewDefaultEntitlementFilter()
	}

	if filter.QueryFilter == nil {
		filter.QueryFilter = types.NewDefaultQueryFilter()
	}

	// Set default sort order if not specified
	if filter.QueryFilter.Sort == nil {
		filter.QueryFilter.Sort = lo.ToPtr("created_at")
		filter.QueryFilter.Order = lo.ToPtr("desc")
	}

	entitlements, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	count, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, err
	}

	response := &dto.ListEntitlementsResponse{
		Items: make([]*dto.EntitlementResponse, len(entitlements)),
	}

	// Create maps to store expanded data
	var featuresByID map[string]*feature.Feature
	var plansByID map[string]*plan.Plan
	var metersByID map[string]*meter.Meter

	if !filter.GetExpand().IsEmpty() {
		if filter.GetExpand().Has(types.ExpandFeatures) {
			// Collect feature IDs
			featureIDs := lo.Map(entitlements, func(e *entitlement.Entitlement, _ int) string {
				return e.FeatureID
			})

			if len(featureIDs) > 0 {
				featureFilter := types.NewNoLimitFeatureFilter()
				featureFilter.FeatureIDs = featureIDs
				features, err := s.featureRepo.List(ctx, featureFilter)
				if err != nil {
					return nil, err
				}

				featuresByID = make(map[string]*feature.Feature, len(features))
				for _, f := range features {
					featuresByID[f.ID] = f
				}

				s.log.Debugw("fetched features for entitlements", "count", len(features))
			}
		}

		if filter.GetExpand().Has(types.ExpandMeters) {
			// Collect meter IDs
			meterIDs := []string{}
			for _, f := range featuresByID {
				meterIDs = append(meterIDs, f.MeterID)
			}

			if len(meterIDs) > 0 {
				meterFilter := types.NewNoLimitMeterFilter()
				meterFilter.MeterIDs = meterIDs
				meters, err := s.meterRepo.List(ctx, meterFilter)
				if err != nil {
					return nil, err
				}

				metersByID = make(map[string]*meter.Meter, len(meters))
				for _, m := range meters {
					metersByID[m.ID] = m
				}

				s.log.Debugw("fetched meters for entitlements", "count", len(meters))
			}
		}

		if filter.GetExpand().Has(types.ExpandPlans) {
			// Collect plan IDs
			planIDs := lo.Map(entitlements, func(e *entitlement.Entitlement, _ int) string {
				return e.PlanID
			})

			if len(planIDs) > 0 {
				planFilter := types.NewNoLimitPlanFilter()
				planFilter.PlanIDs = planIDs
				plans, err := s.planRepo.List(ctx, planFilter)
				if err != nil {
					return nil, err
				}

				plansByID = make(map[string]*plan.Plan, len(plans))
				for _, p := range plans {
					plansByID[p.ID] = p
				}

				s.log.Debugw("fetched plans for entitlements", "count", len(plans))
			}
		}
	}

	for i, e := range entitlements {
		response.Items[i] = &dto.EntitlementResponse{Entitlement: e}

		// Add expanded feature if requested and available
		if !filter.GetExpand().IsEmpty() && filter.GetExpand().Has(types.ExpandFeatures) {
			if f, ok := featuresByID[e.FeatureID]; ok {
				response.Items[i].Feature = &dto.FeatureResponse{Feature: f}
				// Add expanded meter if requested and available
				if filter.GetExpand().Has(types.ExpandMeters) {
					if m, ok := metersByID[f.MeterID]; ok {
						response.Items[i].Feature.Meter = dto.ToMeterResponse(m)
					}
				}
			}
		}

		// Add expanded plan if requested and available
		if !filter.GetExpand().IsEmpty() && filter.GetExpand().Has(types.ExpandPlans) {
			if p, ok := plansByID[e.PlanID]; ok {
				response.Items[i].Plan = &dto.PlanResponse{Plan: p}
			}
		}
	}

	response.Pagination = types.NewPaginationResponse(
		count,
		filter.GetLimit(),
		filter.GetOffset(),
	)

	return response, nil
}

func (s *entitlementService) UpdateEntitlement(ctx context.Context, id string, req dto.UpdateEntitlementRequest) (*dto.EntitlementResponse, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.IsEnabled != nil {
		existing.IsEnabled = *req.IsEnabled
	}
	if req.UsageLimit != nil {
		// If usage limit is 0, set to nil
		// TODO: This is a workaround and might need to be revisited
		if *req.UsageLimit == 0 {
			existing.UsageLimit = nil
		} else {
			existing.UsageLimit = req.UsageLimit
		}
	}
	if req.UsageResetPeriod != "" {
		existing.UsageResetPeriod = req.UsageResetPeriod
	}
	if req.IsSoftLimit != nil {
		existing.IsSoftLimit = *req.IsSoftLimit
	}
	if req.StaticValue != "" {
		existing.StaticValue = req.StaticValue
	}

	// Validate updated entitlement
	if err := existing.Validate(); err != nil {
		return nil, err
	}

	result, err := s.repo.Update(ctx, existing)
	if err != nil {
		return nil, err
	}

	response := &dto.EntitlementResponse{Entitlement: result}
	return response, nil
}

func (s *entitlementService) DeleteEntitlement(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *entitlementService) GetPlanEntitlements(ctx context.Context, planID string) (*dto.ListEntitlementsResponse, error) {
	// Create a filter for the plan's entitlements
	filter := types.NewNoLimitEntitlementFilter()
	filter.WithPlanIDs([]string{planID})
	filter.WithStatus(types.StatusPublished)
	filter.WithExpand(fmt.Sprintf("%s,%s", types.ExpandFeatures, types.ExpandMeters))

	// Use the standard list function to get the entitlements with expansion
	return s.ListEntitlements(ctx, filter)
}

func (s *entitlementService) GetPlanFeatureEntitlements(ctx context.Context, planID, featureID string) (*dto.ListEntitlementsResponse, error) {
	// Create a filter for the feature's entitlements
	filter := types.NewNoLimitEntitlementFilter()
	filter.WithPlanIDs([]string{planID})
	filter.WithFeatureID(featureID)
	filter.WithStatus(types.StatusPublished)
	filter.WithExpand(string(types.ExpandMeters))

	// Use the standard list function to get the entitlements with expansion
	return s.ListEntitlements(ctx, filter)
}
