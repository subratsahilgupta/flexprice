package service

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/svix"
)

// SvixService handles Svix-specific operations
type SvixService interface {
	// GetOrCreateApplication creates a Svix application for a tenant/environment if it doesn't exist.
	// Each tenant/environment combination gets its own Svix application to manage their webhooks independently.
	// Returns the Svix application ID which is used for all subsequent operations.
	GetOrCreateApplication(ctx context.Context, tenantID, environmentID string) (string, error)

	// GetDashboardURL gets the dashboard URL for a Svix application.
	// This URL can be provided to customers to manage their webhook endpoints through Svix's portal.
	// The portal allows customers to:
	// - Add/remove webhook endpoints
	// - View delivery history
	// - Configure retry settings
	// - Access webhook logs
	GetDashboardURL(ctx context.Context, applicationID string) (string, error)

	// SendMessage sends a message to Svix for fan-out delivery.
	// Svix will deliver this message to all configured endpoints for the given application.
	// The eventType parameter categorizes the event (e.g., "invoice.created", "payment.succeeded")
	// The payload is the actual data to be delivered to the endpoints.
	SendMessage(ctx context.Context, applicationID string, eventType string, payload interface{}) error
}

// svixService implements the SvixService interface.
type svixService struct {
	client *svix.Client
	logger *logger.Logger
}

// NewSvixService creates a new Svix service instance with required dependencies
func NewSvixService(
	cfg *config.Configuration,
	logger *logger.Logger,
) (SvixService, error) {
	client, err := svix.NewClient(cfg.Webhook.Svix.AuthToken, cfg.Webhook.Svix.BaseURL, cfg.Webhook.Svix.Enabled)
	if err != nil {
		return nil, fmt.Errorf("failed to create svix client: %w", err)
	}

	return &svixService{
		client: client,
		logger: logger,
	}, nil
}

// GetOrCreateApplication implements the application management workflow
func (s *svixService) GetOrCreateApplication(ctx context.Context, tenantID, environmentID string) (string, error) {
	appID, err := s.client.GetOrCreateApplication(ctx, tenantID, environmentID)
	if err != nil {
		s.logger.Errorw("failed to get/create Svix application",
			"error", err,
			"tenant_id", tenantID,
			"environment_id", environmentID,
		)
		return "", err
	}

	return appID, nil
}

// GetDashboardURL gets the dashboard URL for a Svix application
func (s *svixService) GetDashboardURL(ctx context.Context, applicationID string) (string, error) {
	url, err := s.client.GetDashboardURL(ctx, applicationID)
	if err != nil {
		s.logger.Errorw("failed to get Svix dashboard URL",
			"error", err,
			"application_id", applicationID,
		)
		return "", err
	}

	return url, nil
}

// SendMessage sends a webhook message to the given application
func (s *svixService) SendMessage(ctx context.Context, applicationID string, eventType string, payload interface{}) error {
	if err := s.client.SendMessage(ctx, applicationID, eventType, payload); err != nil {
		s.logger.Errorw("failed to send Svix message",
			"error", err,
			"application_id", applicationID,
			"event_type", eventType,
		)
		return err
	}

	return nil
}
