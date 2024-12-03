package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/google/uuid"
)

type PriceService interface {
	CreatePrice(ctx context.Context, req dto.CreatePriceRequest) (*dto.PriceResponse, error)
	GetPrice(ctx context.Context, id string) (*dto.PriceResponse, error)
	GetPrices(ctx context.Context, filter types.Filter) (*dto.ListPricesResponse, error)
	UpdatePrice(ctx context.Context, id string, req dto.UpdatePriceRequest) (*dto.PriceResponse, error)
	UpdatePriceStatus(ctx context.Context, id string, status types.Status) error
}

type priceService struct {
	repo price.Repository
}

func NewPriceService(repo price.Repository) PriceService {
	return &priceService{repo: repo}
}

func (s *priceService) CreatePrice(ctx context.Context, req dto.CreatePriceRequest) (*dto.PriceResponse, error) {
	price := &price.Price{
		ID:                 uuid.New().String(),
		Amount:             req.Amount,
		Currency:           req.Currency,
		ExternalID:         req.ExternalID,
		ExternalSource:     req.ExternalSource,
		BillingPeriod:      req.BillingPeriod,
		BillingPeriodCount: req.BillingPeriodCount,
		BillingModel:       req.BillingModel,
		BillingCadence:     req.BillingCadence,
		BillingCountryCode: req.BillingCountryCode,
		LookupKey:          req.LookupKey,
		Description:        req.Description,
		Metadata:           req.Metadata,
		TierMode:           req.TierMode,
		Tiers:              req.Tiers,
		Transform:          req.Transform,
		BaseModel: types.BaseModel{
			TenantID:  types.GetTenantID(ctx),
			Status:    types.StatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			CreatedBy: types.GetUserID(ctx),
			UpdatedBy: types.GetUserID(ctx),
		},
	}

	if err := s.repo.CreatePrice(ctx, price); err != nil {
		return nil, fmt.Errorf("failed to create price: %w", err)
	}

	return &dto.PriceResponse{Price: price}, nil
}

func (s *priceService) GetPrice(ctx context.Context, id string) (*dto.PriceResponse, error) {
	price, err := s.repo.GetPrice(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	return &dto.PriceResponse{Price: price}, nil
}

func (s *priceService) GetPrices(ctx context.Context, filter types.Filter) (*dto.ListPricesResponse, error) {
	prices, err := s.repo.GetPrices(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list prices: %w", err)
	}

	response := &dto.ListPricesResponse{
		Prices: make([]dto.PriceResponse, len(prices)),
	}

	for i, p := range prices {
		response.Prices[i] = dto.PriceResponse{Price: p}
	}

	response.Total = len(prices)
	response.Offset = filter.Offset
	response.Limit = filter.Limit

	return response, nil
}

func (s *priceService) UpdatePrice(ctx context.Context, id string, req dto.UpdatePriceRequest) (*dto.PriceResponse, error) {
	price, err := s.repo.GetPrice(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	price.Description = req.Description
	price.Metadata = req.Metadata
	price.UpdatedAt = time.Now().UTC()
	price.UpdatedBy = types.GetUserID(ctx)

	if err := s.repo.UpdatePrice(ctx, price); err != nil {
		return nil, fmt.Errorf("failed to update price: %w", err)
	}

	return &dto.PriceResponse{Price: price}, nil
}

func (s *priceService) UpdatePriceStatus(ctx context.Context, id string, status types.Status) error {
	if err := s.repo.UpdatePriceStatus(ctx, id, status); err != nil {
		return fmt.Errorf("failed to update price status: %w", err)
	}
	return nil
}
