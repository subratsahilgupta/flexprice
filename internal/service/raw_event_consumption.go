package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/events/transform"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/pubsub"
	"github.com/flexprice/flexprice/internal/pubsub/kafka"
	pubsubRouter "github.com/flexprice/flexprice/internal/pubsub/router"
	"github.com/flexprice/flexprice/internal/sentry"
)

// RawEventConsumptionService handles consuming raw event batches from Kafka and transforming them
type RawEventConsumptionService interface {
	// Register message handler with the router
	RegisterHandler(router *pubsubRouter.Router, cfg *config.Configuration)
}

type rawEventConsumptionService struct {
	ServiceParams
	pubSub        pubsub.PubSub
	outputPubSub  pubsub.PubSub
	sentryService *sentry.Service
}

// RawEventBatch represents the batch structure from Bento
type RawEventBatch struct {
	Data          []json.RawMessage `json:"data"`
	TenantID      string            `json:"tenant_id"`
	EnvironmentID string            `json:"environment_id"`
}

// NewRawEventConsumptionService creates a new raw event consumption service
func NewRawEventConsumptionService(
	params ServiceParams,
	sentryService *sentry.Service,
) RawEventConsumptionService {
	ev := &rawEventConsumptionService{
		ServiceParams: params,
		sentryService: sentryService,
	}

	// Consumer pubsub for raw_events topic
	pubSub, err := kafka.NewPubSubFromConfig(
		params.Config,
		params.Logger,
		params.Config.RawEventConsumption.ConsumerGroup,
	)
	if err != nil {
		params.Logger.Fatalw("failed to create pubsub for raw event consumption", "error", err)
		return nil
	}
	ev.pubSub = pubSub

	// Output pubsub for publishing transformed events to events topic
	outputPubSub, err := kafka.NewPubSubFromConfig(
		params.Config,
		params.Logger,
		"raw-event-consumption-producer",
	)
	if err != nil {
		params.Logger.Fatalw("failed to create output pubsub for raw event consumption", "error", err)
		return nil
	}
	ev.outputPubSub = outputPubSub

	return ev
}

// RegisterHandler registers the raw event consumption handler with the router
func (s *rawEventConsumptionService) RegisterHandler(
	router *pubsubRouter.Router,
	cfg *config.Configuration,
) {
	if !cfg.RawEventConsumption.Enabled {
		s.Logger.Infow("raw event consumption handler disabled by configuration")
		return
	}

	// Add throttle middleware to this specific handler
	throttle := middleware.NewThrottle(cfg.RawEventConsumption.RateLimit, time.Second)

	// Add the handler
	router.AddNoPublishHandler(
		"raw_event_consumption_handler",
		cfg.RawEventConsumption.Topic,
		s.pubSub,
		s.processMessage,
		throttle.Middleware,
	)

	s.Logger.Infow("registered raw event consumption handler",
		"topic", cfg.RawEventConsumption.Topic,
		"rate_limit", cfg.RawEventConsumption.RateLimit,
	)
}

// processMessage processes a batch of raw events from Kafka
func (s *rawEventConsumptionService) processMessage(msg *message.Message) error {
	s.Logger.Debugw("processing raw event batch from message queue",
		"message_uuid", msg.UUID,
	)

	// Unmarshal the batch
	var batch RawEventBatch
	if err := json.Unmarshal(msg.Payload, &batch); err != nil {
		s.Logger.Errorw("failed to unmarshal raw event batch",
			"error", err,
			"payload", string(msg.Payload),
		)
		s.sentryService.CaptureException(err)
		return fmt.Errorf("non-retriable unmarshal error: %w", err)
	}

	s.Logger.Infow("processing raw event batch",
		"batch_size", len(batch.Data),
		"message_uuid", msg.UUID,
	)

	// Get tenant and environment IDs from batch payload (priority)
	// Fall back to config if not provided in batch
	tenantID := batch.TenantID
	if tenantID == "" {
		tenantID = s.Config.Billing.TenantID
	}

	environmentID := batch.EnvironmentID
	if environmentID == "" {
		environmentID = s.Config.Billing.EnvironmentID
	}

	s.Logger.Debugw("using tenant and environment context",
		"tenant_id", tenantID,
		"environment_id", environmentID,
		"source", func() string {
			if batch.TenantID != "" {
				return "batch_payload"
			}
			return "config"
		}(),
	)

	// Counters for tracking
	successCount := 0
	skipCount := 0
	errorCount := 0

	// Process each raw event in the batch
	for i, rawEventPayload := range batch.Data {
		// Transform the raw event using existing transformer
		transformedEvent, err := transform.TransformBentoToEvent(
			string(rawEventPayload),
			tenantID,
			environmentID,
		)

		if err != nil {
			// Transformation error
			errorCount++
			s.Logger.Warnw("transformation error - event skipped",
				"batch_position", i+1,
				"error", err.Error(),
			)
			continue
		}

		if transformedEvent == nil {
			// Event failed validation and was dropped
			skipCount++
			s.Logger.Debugw("validation failed - event dropped",
				"batch_position", i+1,
			)
			continue
		}

		// Publish the transformed event to events topic
		if err := s.publishTransformedEvent(context.Background(), transformedEvent); err != nil {
			errorCount++
			s.Logger.Errorw("failed to publish transformed event",
				"event_id", transformedEvent.ID,
				"event_name", transformedEvent.EventName,
				"external_customer_id", transformedEvent.ExternalCustomerID,
				"batch_position", i+1,
				"error", err.Error(),
			)
			// Continue processing other events even if one fails
			continue
		}

		successCount++
		s.Logger.Debugw("successfully transformed and published event",
			"event_id", transformedEvent.ID,
			"event_name", transformedEvent.EventName,
			"batch_position", i+1,
		)
	}

	s.Logger.Infow("completed raw event batch processing",
		"batch_size", len(batch.Data),
		"success_count", successCount,
		"skip_count", skipCount,
		"error_count", errorCount,
		"message_uuid", msg.UUID,
	)

	// Return error only if all events failed
	if successCount == 0 && len(batch.Data) > 0 {
		return fmt.Errorf("failed to process any events in batch")
	}

	return nil
}

// publishTransformedEvent publishes a transformed event to the events topic
func (s *rawEventConsumptionService) publishTransformedEvent(ctx context.Context, event *events.Event) error {
	// Create message payload
	payload, err := json.Marshal(event)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to marshal event for publishing").
			Mark(ierr.ErrValidation)
	}

	// Create a deterministic partition key based on tenant_id and external_customer_id
	partitionKey := event.TenantID
	if event.ExternalCustomerID != "" {
		partitionKey = fmt.Sprintf("%s:%s", event.TenantID, event.ExternalCustomerID)
	}

	// Make UUID truly unique by adding nanosecond precision timestamp and random bytes
	uniqueID := fmt.Sprintf("%s-%d-%d", event.ID, time.Now().UnixNano(), rand.Int63())

	msg := message.NewMessage(uniqueID, payload)

	// Set metadata for additional context
	msg.Metadata.Set("tenant_id", event.TenantID)
	msg.Metadata.Set("environment_id", event.EnvironmentID)
	msg.Metadata.Set("partition_key", partitionKey)

	// Publish to events topic
	topic := s.Config.EventProcessing.Topic

	s.Logger.Debugw("publishing transformed event to kafka",
		"event_id", event.ID,
		"event_name", event.EventName,
		"partition_key", partitionKey,
		"topic", topic,
	)

	// Publish to Kafka
	if err := s.outputPubSub.Publish(ctx, topic, msg); err != nil {
		return ierr.WithError(err).
			WithHint("Failed to publish transformed event to Kafka").
			Mark(ierr.ErrSystem)
	}

	return nil
}
