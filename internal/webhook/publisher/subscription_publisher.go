package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// SubscriptionWebhookPublisher handles publishing of subscription-related webhook events
type SubscriptionWebhookPublisher struct {
	// Configuration for webhook delivery
	webhookURL   string
	maxRetries   int
	retryBackoff time.Duration
	timeout      time.Duration

	// Logging and error tracking
	logger *log.Logger

	// HTTP client for webhook delivery
	httpClient *http.Client

	// Concurrency control
	mu sync.Mutex
}

// WebhookPublisherConfig provides configuration options for the webhook publisher
type WebhookPublisherConfig struct {
	WebhookURL   string
	MaxRetries   int
	RetryBackoff time.Duration
	Timeout      time.Duration
}

// NewSubscriptionWebhookPublisher creates a new webhook publisher
func NewSubscriptionWebhookPublisher(
	config WebhookPublisherConfig,
	logger *log.Logger,
) *SubscriptionWebhookPublisher {
	// Set default values if not provided
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryBackoff == 0 {
		config.RetryBackoff = 1 * time.Second
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	return &SubscriptionWebhookPublisher{
		webhookURL:   config.WebhookURL,
		maxRetries:   config.MaxRetries,
		retryBackoff: config.RetryBackoff,
		timeout:      config.Timeout,
		logger:       logger,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// PublishWebhook sends a webhook event to the configured endpoint
func (p *SubscriptionWebhookPublisher) PublishWebhook(ctx context.Context, event *types.WebhookEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Validate event
	if err := p.validateWebhookEvent(event); err != nil {
		return err
	}

	// Convert event to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		p.logger.Printf("Failed to marshal webhook event: %v", err)
		return fmt.Errorf("failed to marshal webhook event: %w", err)
	}

	// Attempt to publish with retries
	return p.publishWithRetry(ctx, payload)
}

// validateWebhookEvent checks if the webhook event is valid
func (p *SubscriptionWebhookPublisher) validateWebhookEvent(event *types.WebhookEvent) error {
	if event == nil {
		return fmt.Errorf("webhook event cannot be nil")
	}
	if event.EventName == "" {
		return fmt.Errorf("webhook event name cannot be empty")
	}
	return nil
}

// publishWithRetry attempts to publish the webhook with exponential backoff
func (p *SubscriptionWebhookPublisher) publishWithRetry(ctx context.Context, payload []byte) error {
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		// Create a new request
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			p.webhookURL,
			bytes.NewBuffer(payload),
		)
		if err != nil {
			p.logger.Printf("Failed to create webhook request: %v", err)
			return fmt.Errorf("failed to create webhook request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Event", "subscription")

		// Send request
		resp, err := p.httpClient.Do(req)
		if err != nil {
			p.logger.Printf("Webhook delivery attempt %d failed: %v", attempt, err)

			// If it's the last attempt, return the error
			if attempt == p.maxRetries {
				return fmt.Errorf("webhook delivery failed after %d attempts: %w", p.maxRetries, err)
			}

			// Wait before retrying
			time.Sleep(p.retryBackoff * time.Duration(1<<uint(attempt)))
			continue
		}
		defer resp.Body.Close()

		// Check response status
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			p.logger.Printf("Webhook delivery failed with status code %d", resp.StatusCode)

			// If it's the last attempt, return an error
			if attempt == p.maxRetries {
				return fmt.Errorf("webhook delivery failed with status code %d", resp.StatusCode)
			}

			// Wait before retrying
			time.Sleep(p.retryBackoff * time.Duration(1<<uint(attempt)))
			continue
		}

		// Successful delivery
		p.logger.Printf("Webhook event published successfully")
		return nil
	}

	// This should never be reached due to the retry loop, but added for completeness
	return fmt.Errorf("webhook delivery failed after %d attempts", p.maxRetries)
}
