package service

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/types"
)

type PlanService interface {
	CreatePlan(ctx context.Context, req dto.CreatePlanRequest) (*dto.CreatePlanResponse, error)
	GetPlan(ctx context.Context, id string) (*dto.PlanResponse, error)
	GetPlans(ctx context.Context, filter types.Filter) (*dto.ListPlansResponse, error)
	UpdatePlan(ctx context.Context, id string, req dto.UpdatePlanRequest) (*dto.PlanResponse, error)
	DeletePlan(ctx context.Context, id string) error
}

type planService struct {
	planRepo  plan.Repository
	priceRepo price.Repository
}

func NewPlanService(planRepo plan.Repository, priceRepo price.Repository) PlanService {
	return &planService{planRepo: planRepo, priceRepo: priceRepo}
}

func (s *planService) CreatePlan(ctx context.Context, req dto.CreatePlanRequest) (*dto.CreatePlanResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	plan := req.ToPlan(ctx)
	if err := s.planRepo.Create(ctx, plan); err != nil {
		return nil, fmt.Errorf("failed to create plan: %w", err)
	}

	// TODO: Create prices in bulk
	for _, priceReq := range req.Prices {
		price := priceReq.ToPrice(ctx)
		if err := s.priceRepo.Create(ctx, price); err != nil {
			return nil, fmt.Errorf("failed to create price: %w", err)
		}
	}

	return &dto.CreatePlanResponse{Plan: plan}, nil
}

func (s *planService) GetPlan(ctx context.Context, id string) (*dto.PlanResponse, error) {
	plan, err := s.planRepo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	prices, err := s.priceRepo.List(ctx, types.Filter{})
	if err != nil {
		return nil, fmt.Errorf("failed to get prices: %w", err)
	}

	response := &dto.PlanResponse{Plan: plan}
	for _, price := range prices {
		if price.PlanID == plan.ID {
			response.Prices = append(response.Prices, dto.PriceResponse{Price: price})
		}
	}

	return response, nil
}

func (s *planService) GetPlans(ctx context.Context, filter types.Filter) (*dto.ListPlansResponse, error) {
	plans, err := s.planRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get plans: %w", err)
	}

	response := &dto.ListPlansResponse{
		Plans: make([]plan.Plan, len(plans)),
	}

	for i, plan := range plans {
		response.Plans[i] = *plan
	}

	response.Total = len(plans)
	response.Offset = filter.Offset
	response.Limit = filter.Limit

	return response, nil
}

func (s *planService) UpdatePlan(ctx context.Context, id string, req dto.UpdatePlanRequest) (*dto.PlanResponse, error) {
	plan, err := s.GetPlan(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	plan.Name = req.Name
	plan.Description = req.Description

	reqPrices := make(map[string]dto.UpdatePriceRequest)
	for _, price := range req.Prices {
		reqPrices[price.PriceID] = *price.UpdatePriceRequest
	}

	// iterate over prices and update them and remove the ones that are not in the request
	for _, price := range plan.Prices {
		if _, ok := reqPrices[price.ID]; !ok {
			if err := s.priceRepo.Delete(ctx, price.ID); err != nil {
				return nil, fmt.Errorf("failed to delete price: %w", err)
			}
		} else {
			price.Description = reqPrices[price.ID].Description
			price.Metadata = reqPrices[price.ID].Metadata
			if err := s.priceRepo.Update(ctx, price.Price); err != nil {
				return nil, fmt.Errorf("failed to update price: %w", err)
			}
		}
	}

	return plan, nil
}

func (s *planService) DeletePlan(ctx context.Context, id string) error {
	err := s.planRepo.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete plan: %w", err)
	}
	return nil
}
