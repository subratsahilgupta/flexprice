package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/feature"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/publisher"
	"github.com/flexprice/flexprice/internal/pubsub"
	pubsubRouter "github.com/flexprice/flexprice/internal/pubsub/router"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

const (
	OnboardingEventsTopic = "onboarding_events"
)

// OnboardingService handles onboarding-related operations
type OnboardingService interface {
	GenerateEvents(ctx context.Context, req *dto.OnboardingEventsRequest) (*dto.OnboardingEventsResponse, error)
	RegisterHandler()
}

type onboardingService struct {
	eventRepo        events.Repository
	meterRepo        meter.Repository
	customerRepo     customer.Repository
	featureRepo      feature.Repository
	subscriptionRepo subscription.Repository
	priceRepo        price.Repository
	publisher        publisher.EventPublisher
	pubSub           pubsub.PubSub
	pubSubRouter     *pubsubRouter.Router
	logger           *logger.Logger
}

// NewOnboardingService creates a new onboarding service
func NewOnboardingService(
	eventRepo events.Repository,
	customerRepo customer.Repository,
	featureRepo feature.Repository,
	meterRepo meter.Repository,
	subscriptionRepo subscription.Repository,
	priceRepo price.Repository,
	publisher publisher.EventPublisher,
	pubSub pubsub.PubSub,
	pubSubRouter *pubsubRouter.Router,
	logger *logger.Logger,
) OnboardingService {
	return &onboardingService{
		eventRepo:        eventRepo,
		customerRepo:     customerRepo,
		featureRepo:      featureRepo,
		meterRepo:        meterRepo,
		subscriptionRepo: subscriptionRepo,
		priceRepo:        priceRepo,
		publisher:        publisher,
		pubSub:           pubSub,
		pubSubRouter:     pubSubRouter,
		logger:           logger,
	}
}

// GenerateEvents generates events for a specific customer and feature or subscription
func (s *onboardingService) GenerateEvents(ctx context.Context, req *dto.OnboardingEventsRequest) (*dto.OnboardingEventsResponse, error) {
	var customerID string
	meters := make([]types.MeterInfo, 0)
	featureService := NewFeatureService(s.featureRepo, s.meterRepo, s.logger)
	featureFilter := types.NewNoLimitFeatureFilter()
	featureFilter.Expand = lo.ToPtr(string(types.ExpandMeters))

	// If subscription ID is provided, fetch customer and feature information from the subscription
	if req.SubscriptionID != "" {
		// Get subscription
		subscription, subscriptionLineItems, err := s.subscriptionRepo.GetWithLineItems(ctx, req.SubscriptionID)
		if err != nil {
			return nil, fmt.Errorf("failed to get subscription: %w", err)
		}

		// Set customer ID from subscription
		customerID = subscription.CustomerID

		featureFilter.MeterIDs = []string{}
		for _, lineItem := range subscriptionLineItems {
			if lineItem.PriceType == types.PRICE_TYPE_USAGE {
				featureFilter.MeterIDs = append(featureFilter.MeterIDs, lineItem.MeterID)
			}
		}

	} else {
		customerID = req.CustomerID
		featureFilter.FeatureIDs = []string{req.FeatureID}
	}

	customer, err := s.customerRepo.Get(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get customer: %w", err)
	}

	features, err := featureService.GetFeatures(ctx, featureFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to get features: %w", err)
	}

	for _, feature := range features.Items {
		meters = append(meters, createMeterInfoFromMeter(feature.Meter))
	}

	if len(meters) == 0 {
		return nil, fmt.Errorf("no meters found for feature %s", req.FeatureID)
	}

	// Set the customer and feature IDs in the request for logging
	selectedFeature := features.Items[0]
	req.CustomerID = customerID
	req.FeatureID = selectedFeature.ID

	// Create a message with the request details
	msg := &types.OnboardingEventsMessage{
		CustomerID:       customerID,
		CustomerExtID:    customer.ExternalID,
		FeatureID:        selectedFeature.ID,
		FeatureName:      selectedFeature.Name,
		Duration:         req.Duration,
		Meters:           meters,
		TenantID:         types.GetTenantID(ctx),
		RequestTimestamp: time.Now(),
		SubscriptionID:   req.SubscriptionID,
	}

	// Publish the message to the onboarding events topic
	messageID := watermill.NewUUID()
	payload, err := msg.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	watermillMsg := message.NewMessage(messageID, payload)
	watermillMsg.Metadata.Set("tenant_id", types.GetTenantID(ctx))

	s.logger.Infow("publishing onboarding events message",
		"message_id", messageID,
		"customer_id", customerID,
		"feature_id", selectedFeature.ID,
		"subscription_id", req.SubscriptionID,
		"duration", req.Duration,
	)

	if err := s.pubSub.Publish(ctx, OnboardingEventsTopic, watermillMsg); err != nil {
		return nil, fmt.Errorf("failed to publish message: %w", err)
	}

	return &dto.OnboardingEventsResponse{
		Message:        "Event generation started",
		StartedAt:      time.Now(),
		Duration:       req.Duration,
		Count:          req.Duration, // One event per second
		CustomerID:     customerID,
		FeatureID:      selectedFeature.ID,
		SubscriptionID: req.SubscriptionID,
	}, nil
}

// Helper function to create MeterInfo from a Meter
func createMeterInfoFromMeter(m *dto.MeterResponse) types.MeterInfo {
	filterInfos := make([]types.FilterInfo, len(m.Filters))
	for j, f := range m.Filters {
		filterInfos[j] = types.FilterInfo{
			Key:    f.Key,
			Values: f.Values,
		}
	}

	return types.MeterInfo{
		ID:        m.ID,
		EventName: m.EventName,
		Aggregation: types.AggregationInfo{
			Type:  m.Aggregation.Type,
			Field: m.Aggregation.Field,
		},
		Filters: filterInfos,
	}
}

// RegisterHandler registers a handler for onboarding events
func (s *onboardingService) RegisterHandler() {
	s.pubSubRouter.AddNoPublishHandler(
		"onboarding_events_handler",
		OnboardingEventsTopic,
		s.pubSub,
		s.processMessage,
	)
}

// processMessage processes a single onboarding event message
func (s *onboardingService) processMessage(msg *message.Message) error {
	// We don't need the message context anymore since we're using a background context
	// Just log the message UUID for tracing
	s.logger.Debugw("received onboarding event message", "message_uuid", msg.UUID)

	// Unmarshal the message
	var eventMsg types.OnboardingEventsMessage
	if err := eventMsg.Unmarshal(msg.Payload); err != nil {
		s.logger.Errorw("failed to unmarshal onboarding event message",
			"error", err,
			"message_uuid", msg.UUID,
		)
		return nil // Don't retry on unmarshal errors
	}

	s.logger.Infow("processing onboarding events",
		"customer_id", eventMsg.CustomerID,
		"feature_id", eventMsg.FeatureID,
		"subscription_id", eventMsg.SubscriptionID,
		"duration", eventMsg.Duration,
		"meters_count", len(eventMsg.Meters),
	)

	// Create a new background context instead of using the message context
	// This prevents the event generation from being cancelled when the HTTP request completes
	bgCtx := context.Background()

	// Copy tenant ID from original context to background context
	bgCtx = context.WithValue(bgCtx, types.CtxTenantID, eventMsg.TenantID)

	// Start a goroutine to generate events at a rate of 1 per second
	go s.generateEvents(bgCtx, &eventMsg)

	return nil
}

// generateEvents generates events at a rate of 1 per second
func (s *onboardingService) generateEvents(ctx context.Context, eventMsg *types.OnboardingEventsMessage) {
	eventService := NewEventService(s.eventRepo, s.meterRepo, s.publisher, s.logger)

	// Create a ticker to generate events at a rate of 1 per second
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Create a counter for successful events
	successCount := 0
	errorCount := 0

	s.logger.Infow("starting event generation",
		"customer_id", eventMsg.CustomerID,
		"feature_id", eventMsg.FeatureID,
		"duration", eventMsg.Duration,
	)

	// Generate events
	for i := 0; i < eventMsg.Duration; i++ {
		select {
		case <-ticker.C:
			// Generate an event for each meter
			for _, meter := range eventMsg.Meters {
				// Create event request
				eventReq := s.createEventRequest(eventMsg, &meter)

				// Ingest the event
				if err := eventService.CreateEvent(ctx, &eventReq); err != nil {
					errorCount++
					s.logger.Errorw("failed to create event",
						"error", err,
						"customer_id", eventMsg.CustomerID,
						"event_name", meter.EventName,
						"event_number", i+1,
						"total_events", eventMsg.Duration,
					)
					continue
				}

				successCount++
				s.logger.Infow("created onboarding event",
					"customer_id", eventMsg.CustomerID,
					"event_name", meter.EventName,
					"event_id", eventReq.EventID,
					"event_number", i+1,
					"total_events", eventMsg.Duration,
				)
			}
		case <-ctx.Done():
			s.logger.Warnw("context cancelled, stopping event generation",
				"customer_id", eventMsg.CustomerID,
				"feature_id", eventMsg.FeatureID,
				"events_generated", successCount,
				"events_failed", errorCount,
				"total_expected", eventMsg.Duration,
				"reason", ctx.Err(),
			)
			return
		}
	}

	s.logger.Infow("completed generating onboarding events",
		"customer_id", eventMsg.CustomerID,
		"feature_id", eventMsg.FeatureID,
		"duration", eventMsg.Duration,
		"events_generated", successCount,
		"events_failed", errorCount,
	)
}

// createEventRequest creates an event request for a meter
func (s *onboardingService) createEventRequest(eventMsg *types.OnboardingEventsMessage, meter *types.MeterInfo) dto.IngestEventRequest {
	// Generate properties based on meter configuration
	properties := make(map[string]interface{})

	// Handle properties based on meter aggregation and filters
	if meter.Aggregation.Type == types.AggregationSum ||
		meter.Aggregation.Type == types.AggregationCountUnique ||
		meter.Aggregation.Type == types.AggregationAvg {
		// For sum/avg aggregation, we need to generate a value for the aggregation field
		if meter.Aggregation.Field != "" {
			// Generate a random value between 1 and 100
			properties[meter.Aggregation.Field] = rand.Int63n(100) + 1
		}
	}

	// Apply filter values if available
	for _, filter := range meter.Filters {
		if len(filter.Values) > 0 {
			// Select a random value from the filter values
			properties[filter.Key] = filter.Values[rand.Intn(len(filter.Values))]
		}
	}

	return dto.IngestEventRequest{
		EventID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_EVENT),
		ExternalCustomerID: eventMsg.CustomerExtID,
		EventName:          meter.EventName,
		Timestamp:          time.Now(),
		Properties:         properties,
		Source:             "onboarding",
	}
}
