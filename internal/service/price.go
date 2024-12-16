package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type PriceService interface {
	CreatePrice(ctx context.Context, req dto.CreatePriceRequest) (*dto.PriceResponse, error)
	GetPrice(ctx context.Context, id string) (*dto.PriceResponse, error)
	GetPrices(ctx context.Context, filter types.Filter) (*dto.ListPricesResponse, error)
	UpdatePrice(ctx context.Context, id string, req dto.UpdatePriceRequest) (*dto.PriceResponse, error)
	DeletePrice(ctx context.Context, id string) error
	CalculateCost(ctx context.Context, price *price.Price, quantity decimal.Decimal) decimal.Decimal
}

type priceService struct {
	repo   price.Repository
	logger *logger.Logger
}

func NewPriceService(repo price.Repository, logger *logger.Logger) PriceService {
	return &priceService{repo: repo, logger: logger}
}

func (s *priceService) CreatePrice(ctx context.Context, req dto.CreatePriceRequest) (*dto.PriceResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.PlanID == "" {
		return nil, fmt.Errorf("plan_id is required")
	}

	price, err := req.ToPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to parse price: %w", err)
	}

	if err := s.repo.Create(ctx, price); err != nil {
		return nil, fmt.Errorf("failed to create price: %w", err)
	}

	return &dto.PriceResponse{Price: price}, nil
}

func (s *priceService) GetPrice(ctx context.Context, id string) (*dto.PriceResponse, error) {
	price, err := s.repo.Get(ctx, id)
	if err != nil {

		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	return &dto.PriceResponse{Price: price}, nil
}

func (s *priceService) GetPrices(ctx context.Context, filter types.Filter) (*dto.ListPricesResponse, error) {
	prices, err := s.repo.List(ctx, filter)
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
	price, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	price.Description = req.Description
	price.Metadata = req.Metadata
	price.LookupKey = req.LookupKey

	if err := s.repo.Update(ctx, price); err != nil {
		return nil, fmt.Errorf("failed to update price: %w", err)
	}

	return &dto.PriceResponse{Price: price}, nil
}

func (s *priceService) DeletePrice(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete price: %w", err)
	}
	return nil
}

// CalculateCost calculates the cost for a given price and usage
// returns the cost in main currency units (e.g., 1.00 = $1.00)
func (s *priceService) CalculateCost(ctx context.Context, price *price.Price, quantity decimal.Decimal) decimal.Decimal {
	cost := decimal.Zero
	if quantity.IsZero() {
		return cost
	}

	switch price.BillingModel {
	case types.BILLING_MODEL_FLAT_FEE:
		cost = price.CalculateAmountWithPrecision(quantity)

	case types.BILLING_MODEL_PACKAGE:
		if price.TransformQuantity.DivideBy <= 0 {
			return decimal.Zero
		}

		transformedQuantity := quantity.Div(decimal.NewFromInt(int64(price.TransformQuantity.DivideBy)))

		if price.TransformQuantity.Round == types.ROUND_UP {
			transformedQuantity = transformedQuantity.Ceil()
		} else if price.TransformQuantity.Round == types.ROUND_DOWN {
			transformedQuantity = transformedQuantity.Floor()
		}

		cost = price.CalculateAmountWithPrecision(transformedQuantity)

	case types.BILLING_MODEL_TIERED:
		cost = s.calculateTieredCost(ctx, price, quantity)
	}

	finalCost := cost.Round(types.GetCurrencyPrecision(price.Currency))
	return finalCost
}

// calculateTieredCost calculates cost for tiered pricing
func (s *priceService) calculateTieredCost(ctx context.Context, price *price.Price, quantity decimal.Decimal) decimal.Decimal {
	cost := decimal.Zero
	if len(price.Tiers) == 0 {
		s.logger.WithContext(ctx).Errorf("no tiers found for price %s", price.ID)
		return cost
	}

	// Sort price tiers by up_to value
	sort.Slice(price.Tiers, func(i, j int) bool {
		return price.Tiers[i].GetTierUpTo() < price.Tiers[j].GetTierUpTo()
	})

	switch price.TierMode {
	case types.BILLING_TIER_VOLUME:
		selectedTierIndex := len(price.Tiers) - 1
		// Find the tier that the quantity falls into
		for i, tier := range price.Tiers {
			if tier.UpTo == nil {
				selectedTierIndex = i
				break
			}
			if quantity.LessThan(decimal.NewFromUint64(*tier.UpTo)) {
				selectedTierIndex = i
				break
			}
		}

		selectedTier := price.Tiers[selectedTierIndex]

		// Calculate tier cost with proper rounding and handling of flat amount
		tierCost := selectedTier.CalculateTierAmountWithPrecision(quantity, price.Currency)

		s.logger.WithContext(ctx).Debugf(
			"volume tier total cost for quantity %s: %s price: %s tier : %+v",
			quantity.String(),
			tierCost.String(),
			price.ID,
			selectedTier,
		)

		cost = cost.Add(tierCost)

	case types.BILLING_TIER_SLAB:
		remainingQuantity := quantity
		for _, tier := range price.Tiers {
			var tierQuantity = remainingQuantity
			if tier.UpTo != nil {
				upTo := decimal.NewFromUint64(*tier.UpTo)
				if remainingQuantity.GreaterThan(upTo) {
					tierQuantity = upTo
				}
			}

			// Calculate tier cost with proper rounding and handling of flat amount
			tierCost := tier.CalculateTierAmountWithPrecision(tierQuantity, price.Currency)
			cost = cost.Add(tierCost)
			remainingQuantity = remainingQuantity.Sub(tierQuantity)

			s.logger.WithContext(ctx).Debugf(
				"slab tier total cost for quantity %s: %s price: %s tier : %+v",
				quantity.String(),
				tierCost.String(),
				price.ID,
				tier,
			)

			if remainingQuantity.LessThanOrEqual(decimal.Zero) {
				break
			}
		}
	default:
		s.logger.WithContext(ctx).Errorf("invalid tier mode: %s", price.TierMode)
		return decimal.Zero
	}

	return cost
}
