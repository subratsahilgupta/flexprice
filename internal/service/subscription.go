package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type SubscriptionService interface {
	CreateSubscription(ctx context.Context, req dto.CreateSubscriptionRequest) (*dto.SubscriptionResponse, error)
	GetSubscription(ctx context.Context, id string) (*dto.SubscriptionResponse, error)
	CancelSubscription(ctx context.Context, id string, cancelAtPeriodEnd bool) error
	ListSubscriptions(ctx context.Context, filter *types.SubscriptionFilter) (*dto.ListSubscriptionsResponse, error)
	GetUsageBySubscription(ctx context.Context, req *dto.GetUsageBySubscriptionRequest) (*dto.GetUsageBySubscriptionResponse, error)
}

type subscriptionService struct {
	subscriptionRepo subscription.Repository
	planRepo         plan.Repository
	priceRepo        price.Repository
	producer         kafka.MessageProducer
	eventRepo        events.Repository
	meterRepo        meter.Repository
	customerRepo     customer.Repository
	logger           *logger.Logger
}

func NewSubscriptionService(
	subscriptionRepo subscription.Repository,
	planRepo plan.Repository,
	priceRepo price.Repository,
	producer kafka.MessageProducer,
	eventRepo events.Repository,
	meterRepo meter.Repository,
	customerRepo customer.Repository,
	logger *logger.Logger,
) SubscriptionService {
	return &subscriptionService{
		subscriptionRepo: subscriptionRepo,
		planRepo:         planRepo,
		priceRepo:        priceRepo,
		producer:         producer,
		eventRepo:        eventRepo,
		meterRepo:        meterRepo,
		customerRepo:     customerRepo,
		logger:           logger,
	}
}

// CreateSubscription creates a new subscription
// TODO: Add validations and trial logic
func (s *subscriptionService) CreateSubscription(ctx context.Context, req dto.CreateSubscriptionRequest) (*dto.SubscriptionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	plan, err := s.planRepo.Get(ctx, req.PlanID)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	if plan.Status != types.StatusPublished {
		return nil, fmt.Errorf("plan is not active")
	}

	prices, err := s.priceRepo.GetByPlanID(ctx, req.PlanID)
	if err != nil {
		return nil, fmt.Errorf("failed to get prices: %w", err)
	}

	if len(prices) == 0 {
		return nil, fmt.Errorf("no prices found for plan")
	}

	subscription := req.ToSubscription(ctx)
	now := time.Now().UTC()
	if subscription.StartDate.IsZero() {
		subscription.StartDate = now
	}

	if subscription.BillingAnchor.IsZero() {
		subscription.BillingAnchor = subscription.StartDate
	}

	if subscription.BillingPeriodCount == 0 {
		subscription.BillingPeriodCount = 1
	}

	nextBillingDate, err := types.NextBillingDate(subscription.StartDate, subscription.BillingPeriodCount, subscription.BillingPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate next billing date: %w", err)
	}

	subscription.CurrentPeriodStart = subscription.StartDate
	subscription.CurrentPeriodEnd = nextBillingDate
	subscription.InvoiceCadence = plan.InvoiceCadence
	subscription.Currency = prices[0].Currency

	if err := s.subscriptionRepo.Create(ctx, subscription); err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	return &dto.SubscriptionResponse{Subscription: subscription}, nil
}

func (s *subscriptionService) GetSubscription(ctx context.Context, id string) (*dto.SubscriptionResponse, error) {
	subscription, err := s.subscriptionRepo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	// TODO: Think of a better way to handle this initialization of other services
	planService := NewPlanService(s.planRepo, s.priceRepo, s.logger)

	plan, err := planService.GetPlan(ctx, subscription.PlanID)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	return &dto.SubscriptionResponse{Subscription: subscription, Plan: plan}, nil
}

func (s *subscriptionService) CancelSubscription(ctx context.Context, id string, cancelAtPeriodEnd bool) error {
	subscription, err := s.subscriptionRepo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	now := time.Now().UTC()
	subscription.SubscriptionStatus = types.SubscriptionStatusCancelled
	subscription.CancelledAt = &now
	subscription.CancelAtPeriodEnd = cancelAtPeriodEnd

	if err := s.subscriptionRepo.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}

	return nil
}

func (s *subscriptionService) ListSubscriptions(ctx context.Context, filter *types.SubscriptionFilter) (*dto.ListSubscriptionsResponse, error) {
	if filter.Limit == 0 {
		filter.Limit = 10
	}

	subscriptions, err := s.subscriptionRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: %w", err)
	}

	response := &dto.ListSubscriptionsResponse{
		Subscriptions: make([]*dto.SubscriptionResponse, len(subscriptions)),
		Total:         len(subscriptions),
		Offset:        filter.Offset,
		Limit:         filter.Limit,
	}

	for i, sub := range subscriptions {
		plan, err := s.planRepo.Get(ctx, sub.PlanID)
		if err != nil {
			return nil, fmt.Errorf("failed to get plan: %w", err)
		}

		response.Subscriptions[i] = &dto.SubscriptionResponse{
			Subscription: sub,
			Plan: &dto.PlanResponse{
				Plan: plan,
			},
		}
	}

	return response, nil
}

func (s *subscriptionService) GetUsageBySubscription(ctx context.Context, req *dto.GetUsageBySubscriptionRequest) (*dto.GetUsageBySubscriptionResponse, error) {
	response := &dto.GetUsageBySubscriptionResponse{}

	eventService := NewEventService(s.producer, s.eventRepo, s.meterRepo, s.logger)
	priceService := NewPriceService(s.priceRepo, s.logger)

	subscriptionResponse, err := s.GetSubscription(ctx, req.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	subscription := subscriptionResponse.Subscription
	plan := subscriptionResponse.Plan
	pricesResponse := plan.Prices

	// Filter only the eligible prices
	pricesResponse = filterValidPricesForSubscription(pricesResponse, subscription)

	customer, err := s.customerRepo.Get(ctx, subscription.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get customer: %w", err)
	}

	usageStartTime := req.StartTime
	if usageStartTime.IsZero() {
		usageStartTime = subscription.CurrentPeriodStart
	}

	usageEndTime := req.EndTime
	if usageEndTime.IsZero() {
		usageEndTime = subscription.CurrentPeriodEnd
	}

	// saved meter display names
	meterDisplayNames := make(map[string]string)

	// Group prices by meter ID to handle default and filtered prices
	meterPrices := make(map[string][]dto.PriceResponse)
	for _, priceResponse := range pricesResponse {
		if priceResponse.Price.Type != types.PRICE_TYPE_USAGE {
			continue
		}

		meterID := priceResponse.Price.MeterID
		meterPrices[meterID] = append(meterPrices[meterID], priceResponse)
	}

	// Pre-fetch all meter display names
	for meterID := range meterPrices {
		meterDisplayNames[meterID] = getMeterDisplayName(ctx, s.meterRepo, meterID, meterDisplayNames)
	}

	totalCost := decimal.Zero
	// Process each meter's usage separately

	s.logger.Debugw("calculating usage for subscription",
		"subscription_id", req.SubscriptionID,
		"start_time", usageStartTime,
		"end_time", usageEndTime,
		"num_prices", len(pricesResponse))

	for meterID, meterPriceGroup := range meterPrices {
		// Sort prices by filter count to make sure narrower filters are processed first
		sort.Slice(meterPriceGroup, func(i, j int) bool {
			return len(meterPriceGroup[i].Price.FilterValues) > len(meterPriceGroup[j].Price.FilterValues)
		})

		filterGroups := make(map[string]map[string][]string, 0)
		for _, p := range meterPriceGroup {
			filterGroups[p.Price.ID] = p.Price.FilterValues
		}

		usages, err := eventService.GetUsageByMeterWithFilters(ctx, &dto.GetUsageByMeterRequest{
			MeterID:            meterID,
			ExternalCustomerID: customer.ExternalID,
			StartTime:          usageStartTime,
			EndTime:            usageEndTime,
		}, filterGroups)
		if err != nil {
			return nil, fmt.Errorf("failed to get usage for meter %s: %w", meterID, err)
		}

		for _, priceResponse := range meterPriceGroup {
			var quantity decimal.Decimal

			var matchingUsage *events.AggregationResult
			for _, usage := range usages {
				if fgID, ok := usage.Metadata["filter_group_id"]; ok && fgID == priceResponse.Price.ID {
					matchingUsage = usage
					break
				}
			}

			if matchingUsage != nil {
				quantity = matchingUsage.Value
				cost := priceService.CalculateCost(ctx, priceResponse.Price, quantity)
				totalCost = totalCost.Add(cost)

				s.logger.Debugw("calculated usage for meter",
					"meter_id", meterID,
					"quantity", quantity,
					"cost", cost,
					"total_cost", totalCost,
					"meter_display_name", meterDisplayNames[meterID],
					"subscription_id", req.SubscriptionID,
					"usage", matchingUsage,
					"price", priceResponse.Price,
					"filter_values", priceResponse.Price.FilterValues,
				)

				filteredUsageCharge := createChargeResponse(
					priceResponse.Price,
					quantity,
					cost,
					meterDisplayNames[meterID],
				)

				if filteredUsageCharge == nil {
					continue
				}

				if filteredUsageCharge.Quantity > 0 && filteredUsageCharge.Amount > 0 {
					response.Charges = append(response.Charges, filteredUsageCharge)
				}
			}
		}
	}

	response.StartTime = usageStartTime
	response.EndTime = usageEndTime
	response.Amount = price.FormatAmountToFloat64WithPrecision(totalCost, subscription.Currency)
	response.Currency = subscription.Currency
	response.DisplayAmount = price.GetDisplayAmountWithPrecision(totalCost, subscription.Currency)

	return response, nil
}

func createChargeResponse(priceObj *price.Price, quantity decimal.Decimal, cost decimal.Decimal, meterDisplayName string) *dto.SubscriptionUsageByMetersResponse {
	finalAmount := price.FormatAmountToFloat64WithPrecision(cost, priceObj.Currency)
	if finalAmount <= 0 {
		return nil
	}

	return &dto.SubscriptionUsageByMetersResponse{
		Amount:           finalAmount,
		Currency:         priceObj.Currency,
		DisplayAmount:    price.GetDisplayAmountWithPrecision(cost, priceObj.Currency),
		Quantity:         quantity.InexactFloat64(),
		FilterValues:     priceObj.FilterValues,
		MeterDisplayName: meterDisplayName,
		Price:            priceObj,
	}
}

func getMeterDisplayName(ctx context.Context, meterRepo meter.Repository, meterID string, cache map[string]string) string {
	if name, ok := cache[meterID]; ok {
		return name
	}

	meter, err := meterRepo.GetMeter(ctx, meterID)
	if err != nil {
		return meterID // Fallback to meterID if error
	}

	displayName := meter.Name
	if displayName == "" {
		displayName = meter.EventName
	}
	cache[meterID] = displayName
	return displayName
}

func filterValidPricesForSubscription(prices []dto.PriceResponse, subscriptionObj *subscription.Subscription) []dto.PriceResponse {
	var validPrices []dto.PriceResponse

	for _, price := range prices {
		// filter by currency
		if price.Price.Currency == subscriptionObj.Currency {
			// filter by billing period
			if price.Price.BillingPeriod == subscriptionObj.BillingPeriod {
				// filter by billing period count
				if price.Price.BillingPeriodCount == subscriptionObj.BillingPeriodCount {
					validPrices = append(validPrices, price)
				}
			}
		}
	}
	return validPrices
}
