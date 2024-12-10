package service

import (
	"context"
	"fmt"
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

	if subscription.BillingPeriodUnit == 0 {
		subscription.BillingPeriodUnit = 1
	}

	nextBillingDate, err := types.NextBillingDate(subscription.StartDate, subscription.BillingPeriodUnit, subscription.BillingPeriod)
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
	meterPrices := make(map[string][]*dto.PriceResponse)
	for _, priceResponse := range pricesResponse {
		if priceResponse.Price.Type != types.PRICE_TYPE_USAGE {
			continue
		}

		meterID := priceResponse.Price.MeterID
		meterPrices[meterID] = append(meterPrices[meterID], &priceResponse)
	}

	totalCost := uint64(0)
	// Process each meter's usage separately
	for meterID, prices := range meterPrices {
		// Get total usage for the meter
		totalUsage, err := eventService.GetUsageByMeter(ctx, &dto.GetUsageByMeterRequest{
			MeterID:            meterID,
			ExternalCustomerID: customer.ExternalID,
			StartTime:          usageStartTime,
			EndTime:            usageEndTime,
			// Don't apply filters here - we'll handle filtering in memory
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get usage for meter %s: %w", meterID, err)
		}

		// Find default price (price without filters)
		var defaultPrice *dto.PriceResponse
		var filteredPrices []*dto.PriceResponse
		for _, p := range prices {
			if len(p.Price.FilterValues) == 0 {
				defaultPrice = p
			} else {
				filteredPrices = append(filteredPrices, p)
			}
		}

		// Process filtered prices first
		remainingUsage := totalUsage.Value // Keep track of usage not matched by filters
		for _, priceObj := range filteredPrices {
			filteredUsage, err := eventService.GetUsageByMeter(ctx, &dto.GetUsageByMeterRequest{
				MeterID:            meterID,
				ExternalCustomerID: customer.ExternalID,
				StartTime:          usageStartTime,
				EndTime:            usageEndTime,
				Filters:            priceObj.Price.FilterValues,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get filtered usage for meter %s: %w", meterID, err)
			}

			// Subtract filtered usage from remaining usage
			remainingUsage = subtractUsage(remainingUsage, filteredUsage.Value)

			cost := priceService.CalculateCost(ctx, priceObj.Price, filteredUsage)
			totalCost += cost

			filteredUsageCharge := createChargeResponse(
				priceObj.Price,
				filteredUsage,
				cost,
				getMeterDisplayName(ctx, s, meterID, meterDisplayNames),
			)

			if filteredUsageCharge.Quantity > 0 && filteredUsageCharge.Amount > 0 {
				response.Charges = append(response.Charges, filteredUsageCharge)
			}
		}

		// Apply default price to remaining usage if it exists
		if defaultPrice != nil && !isZeroUsage(remainingUsage) {
			defaultUsage := &events.AggregationResult{
				Value:     remainingUsage,
				EventName: totalUsage.EventName,
			}
			cost := priceService.CalculateCost(ctx, defaultPrice.Price, defaultUsage)
			totalCost += cost

			defaultUsageCharge := createChargeResponse(
				defaultPrice.Price,
				defaultUsage,
				cost,
				getMeterDisplayName(ctx, s, meterID, meterDisplayNames),
			)

			if defaultUsageCharge.Quantity > 0 && defaultUsageCharge.Amount > 0 {
				response.Charges = append(response.Charges, defaultUsageCharge)
			}
		}
	}

	response.StartTime = usageStartTime
	response.EndTime = usageEndTime
	response.Amount = price.GetAmountInDollars(totalCost)
	response.Currency = subscription.Currency
	response.DisplayAmount = price.GetDisplayAmount(totalCost, subscription.Currency)

	return response, nil
}

// Helper functions
func subtractUsage(total, subtract interface{}) interface{} {
	switch t := total.(type) {
	case float64:
		if s, ok := subtract.(float64); ok {
			return t - s
		}
	case uint64:
		if s, ok := subtract.(uint64); ok {
			return t - s
		}
	}
	return total
}

func isZeroUsage(usage interface{}) bool {
	switch v := usage.(type) {
	case float64:
		return v == 0
	case uint64:
		return v == 0
	default:
		return true
	}
}

func createChargeResponse(priceObj *price.Price, usage *events.AggregationResult, cost uint64, meterDisplayName string) *dto.SubscriptionUsageByMetersResponse {
	var quantity float64
	switch v := usage.Value.(type) {
	case float64:
		quantity = v
	case uint64:
		quantity = float64(v)
	}

	return &dto.SubscriptionUsageByMetersResponse{
		Amount:           price.GetAmountInDollars(cost),
		Currency:         priceObj.Currency,
		DisplayAmount:    price.GetDisplayAmount(cost, priceObj.Currency),
		Quantity:         quantity,
		FilterValues:     priceObj.FilterValues,
		MeterDisplayName: meterDisplayName,
	}
}

func getMeterDisplayName(ctx context.Context, s *subscriptionService, meterID string, cache map[string]string) string {
	if name, ok := cache[meterID]; ok {
		return name
	}

	meter, err := s.meterRepo.GetMeter(ctx, meterID)
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
