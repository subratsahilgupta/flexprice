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
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/publisher"
	"github.com/flexprice/flexprice/internal/pubsub"
	pubsubRouter "github.com/flexprice/flexprice/internal/pubsub/router"
	"github.com/flexprice/flexprice/internal/types"
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
	eventRepo    events.Repository
	meterRepo    meter.Repository
	customerRepo customer.Repository
	featureRepo  feature.Repository
	publisher    publisher.EventPublisher
	pubSub       pubsub.PubSub
	pubSubRouter *pubsubRouter.Router
	logger       *logger.Logger
}

// NewOnboardingService creates a new onboarding service
func NewOnboardingService(
	eventRepo events.Repository,
	customerRepo customer.Repository,
	featureRepo feature.Repository,
	meterRepo meter.Repository,
	publisher publisher.EventPublisher,
	pubSub pubsub.PubSub,
	pubSubRouter *pubsubRouter.Router,
	logger *logger.Logger,
) OnboardingService {
	return &onboardingService{
		eventRepo:    eventRepo,
		customerRepo: customerRepo,
		featureRepo:  featureRepo,
		meterRepo:    meterRepo,
		publisher:    publisher,
		pubSub:       pubSub,
		pubSubRouter: pubSubRouter,
		logger:       logger,
	}
}

// GenerateEvents generates events for a specific customer and feature
func (s *onboardingService) GenerateEvents(ctx context.Context, req *dto.OnboardingEventsRequest) (*dto.OnboardingEventsResponse, error) {
	// Validate customer exists
	cust, err := s.customerRepo.Get(ctx, req.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get customer: %w", err)
	}

	// Validate feature exists
	feat, err := s.featureRepo.Get(ctx, req.FeatureID)
	if err != nil {
		return nil, fmt.Errorf("failed to get feature: %w", err)
	}

	// Get all meters - in a real implementation, you'd filter by feature
	meters, err := s.meterRepo.List(ctx, &types.MeterFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to get meters: %w", err)
	}

	if len(meters) == 0 {
		return nil, fmt.Errorf("no meters found")
	}

	// For simplicity, just use the first meter
	// In a real implementation, you'd use meters associated with the feature
	meter := meters[0]

	// Create a simplified MeterInfo
	meterInfo := types.MeterInfo{
		ID:        meter.ID,
		EventName: meter.EventName,
		Aggregation: types.AggregationInfo{
			Type:  meter.Aggregation.Type,
			Field: meter.Aggregation.Field,
		},
		Filters: []types.FilterInfo{},
	}

	// Add filters if any
	for _, f := range meter.Filters {
		meterInfo.Filters = append(meterInfo.Filters, types.FilterInfo{
			Key:    f.Key,
			Values: f.Values,
		})
	}

	// Create a message with the request details
	msg := &types.OnboardingEventsMessage{
		CustomerID:       req.CustomerID,
		CustomerExtID:    cust.ExternalID,
		FeatureID:        req.FeatureID,
		FeatureName:      feat.Name,
		Duration:         req.Duration,
		Meters:           []types.MeterInfo{meterInfo},
		TenantID:         types.GetTenantID(ctx),
		RequestTimestamp: time.Now(),
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
		"customer_id", req.CustomerID,
		"feature_id", req.FeatureID,
		"duration", req.Duration,
	)

	if err := s.pubSub.Publish(ctx, OnboardingEventsTopic, watermillMsg); err != nil {
		return nil, fmt.Errorf("failed to publish message: %w", err)
	}

	return &dto.OnboardingEventsResponse{
		Message:   "Event generation started",
		StartedAt: time.Now(),
		Duration:  req.Duration,
		Count:     req.Duration, // One event per second
	}, nil
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

	// Create a done channel to signal completion
	done := make(chan struct{})
	defer close(done)

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
		case <-done:
			// This case should never be reached, but it's here for safety
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
