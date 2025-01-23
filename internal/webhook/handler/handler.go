package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/httpclient"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/pubsub"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/webhook/payload"
)

// Handler interface for processing webhook events
type Handler interface {
	HandleWebhookEvents(ctx context.Context) error
	Close() error
}

// handler implements handler.Handler using watermill's gochannel
type handler struct {
	pubSub  pubsub.PubSub
	config  *config.Webhook
	factory payload.PayloadBuilderFactory
	client  httpclient.Client
	logger  *logger.Logger
	cancel  context.CancelFunc // Add cancel function
}

// NewHandler creates a new memory-based handler
func NewHandler(
	pubSub pubsub.PubSub,
	cfg *config.Configuration,
	factory payload.PayloadBuilderFactory,
	client httpclient.Client,
	logger *logger.Logger,
) (Handler, error) {
	return &handler{
		pubSub:  pubSub,
		config:  &cfg.Webhook,
		factory: factory,
		client:  client,
		logger:  logger,
	}, nil
}

// HandleWebhookEvents starts handling webhook events
func (h *handler) HandleWebhookEvents(c context.Context) error {
	// Create a new context that we can cancel when stopping
	ctx, cancel := context.WithCancel(c)
	h.cancel = cancel

	h.logger.Debugw("subscribing to webhook events", "topic", h.config.Topic)

	messages, err := h.pubSub.Subscribe(ctx, h.config.Topic)
	if err != nil {
		cancel() // Clean up if subscribe fails
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	h.logger.Infow("successfully subscribed to webhook events", "topic", h.config.Topic)

	// Start processing in a goroutine
	go func() {
		h.logger.Debug("starting webhook event processing loop")
		defer h.logger.Info("webhook event processing loop stopped")
		defer cancel() // Ensure context is cancelled when loop exits

		for {
			select {
			case <-ctx.Done():
				h.logger.Debug("context cancelled, stopping webhook processing")
				return
			case msg, ok := <-messages:
				if !ok {
					h.logger.Warn("message channel closed")
					return
				}

				h.logger.Debugw("received webhook message",
					"message_uuid", msg.UUID,
					"metadata", msg.Metadata,
				)

				// Create message context with timeout
				msgCtx, msgCancel := context.WithTimeout(ctx, 30*time.Second)

				// Process message
				if err := h.processMessage(msgCtx, msg); err != nil {
					h.logger.Errorw("failed to process webhook message",
						"error", err,
						"message_uuid", msg.UUID,
					)
					msg.Nack()
				} else {
					msg.Ack()
				}

				msgCancel()
			}
		}
	}()

	return nil
}

// processMessage processes a single webhook message
func (h *handler) processMessage(ctx context.Context, msg *message.Message) error {
	var event types.WebhookEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		h.logger.Errorw("failed to unmarshal webhook event",
			"error", err,
			"message_uuid", msg.UUID,
		)
		// Don't retry on unmarshal errors
		return nil
	}

	// Get tenant config
	tenantCfg, ok := h.config.Tenants[event.TenantID]
	if !ok {
		h.logger.Warnw("tenant config not found",
			"tenant_id", event.TenantID,
			"message_uuid", msg.UUID,
		)
		// Don't retry if tenant not found
		return nil
	}

	// Check if tenant webhooks are enabled
	if !tenantCfg.Enabled {
		h.logger.Debugw("webhooks disabled for tenant",
			"tenant_id", event.TenantID,
			"message_uuid", msg.UUID,
		)
		return nil
	}

	// Check if event is excluded
	for _, excludedEvent := range tenantCfg.ExcludedEvents {
		if excludedEvent == event.EventName {
			h.logger.Debugw("event excluded for tenant",
				"tenant_id", event.TenantID,
				"event", event.EventName,
			)
			return nil
		}
	}

	// Build event payload
	builder, err := h.factory.GetBuilder(event.EventName)
	if err != nil {
		return err
	}

	h.logger.Debugw("building webhook payload",
		"event_name", event.EventName,
		"builder", builder,
	)

	webHookPayload, err := builder.BuildPayload(ctx, event.EventName, event.Payload)
	if err != nil {
		return err
	}

	h.logger.Debugw("built webhook payload",
		"event_name", event.EventName,
		"payload", string(webHookPayload),
	)

	// Send webhook
	req := &httpclient.Request{
		Method:  "POST",
		URL:     tenantCfg.Endpoint,
		Headers: tenantCfg.Headers,
		Body:    webHookPayload,
	}

	resp, err := h.client.Send(ctx, req)
	if err != nil {
		h.logger.Errorw("failed to send webhook",
			"error", err,
			"message_uuid", msg.UUID,
			"tenant_id", event.TenantID,
			"event", event.EventName,
		)
		// Return error to trigger retry
		return fmt.Errorf("failed to send webhook: %w", err)
	}

	if resp.StatusCode >= 400 {
		h.logger.Errorw("webhook request failed",
			"status_code", resp.StatusCode,
			"message_uuid", msg.UUID,
			"tenant_id", event.TenantID,
			"event", event.EventName,
			"error", string(resp.Body),
		)
		// Return error to trigger retry
		return fmt.Errorf("webhook request failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	h.logger.Infow("webhook sent successfully",
		"message_uuid", msg.UUID,
		"tenant_id", event.TenantID,
		"event", event.EventName,
		"status_code", resp.StatusCode,
	)

	return nil
}

// Close closes the handler
func (h *handler) Close() error {
	h.logger.Info("closing webhook handler")
	if h.cancel != nil {
		h.cancel()
	}
	return h.pubSub.Close()
}
