package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/httpclient"
	"github.com/flexprice/flexprice/internal/logger"
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
// It requires:
// - Configuration with Svix credentials and base URL
// - HTTP client for making API calls
// - Logger for tracking operations
type svixService struct {
	config *config.Webhook   // Contains Svix auth token and base URL
	client httpclient.Client // HTTP client for making API calls
	logger *logger.Logger    // Logger for operation tracking
}

// NewSvixService creates a new Svix service instance with required dependencies
func NewSvixService(
	cfg *config.Configuration,
	client httpclient.Client,
	logger *logger.Logger,
) SvixService {
	return &svixService{
		config: &cfg.Webhook,
		client: client,
		logger: logger,
	}
}

// GetOrCreateApplication implements the application management workflow:
// 1. Attempts to fetch an existing application using GET /api/v1/app/{appId}
// 2. If not found, creates a new application using POST /api/v1/app
//
// The appID is constructed as "{tenantID}_{environmentID}" to ensure uniqueness
// and proper isolation between environments.
//
// API Endpoints Used:
// - GET  {baseURL}/api/v1/app/{appId} - Fetch existing application
// - POST {baseURL}/api/v1/app         - Create new application
//
// The base URL is configured in your Svix dashboard and typically follows the format:
// - Production: https://api.svix.com
// - Development: https://api.us-east-1.svix.com or similar regional endpoints
func (s *svixService) GetOrCreateApplication(ctx context.Context, tenantID, environmentID string) (string, error) {
	// First try to get existing application
	appID := fmt.Sprintf("%s_%s", tenantID, environmentID)

	req := &httpclient.Request{
		Method: http.MethodGet,
		URL:    fmt.Sprintf("%s/api/v1/app/%s", s.config.Svix.BaseURL, appID),
		Headers: map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", s.config.Svix.AuthToken),
		},
	}

	resp, err := s.client.Send(ctx, req)
	if err == nil {
		// Application exists - parse and return its ID
		var result struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			return "", fmt.Errorf("failed to decode Svix response: %w", err)
		}
		return result.ID, nil
	}

	// Check if error is due to application not found
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("application not found")
	}

	// Create new application with POST request
	// The name and uid are set to the appID for consistency
	createReq := &httpclient.Request{
		Method: http.MethodPost,
		URL:    fmt.Sprintf("%s/api/v1/app", s.config.Svix.BaseURL),
		Headers: map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", s.config.Svix.AuthToken),
			"Content-Type":  "application/json",
		},
		Body: []byte(fmt.Sprintf(`{"name":"%s","uid":"%s"}`, appID, appID)),
	}

	createResp, err := s.client.Send(ctx, createReq)
	if err != nil {
		return "", fmt.Errorf("failed to create Svix application: %w", err)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createResp.Body, &result); err != nil {
		return "", fmt.Errorf("failed to decode Svix response: %w", err)
	}

	return result.ID, nil
}

// GetDashboardURL generates a portal access URL for the specified application.
// This URL provides temporary access to the Svix dashboard for webhook management.
//
// API Endpoint Used:
// - POST {baseURL}/api/v1/app/{applicationID}/portal-token
//
// The returned URL format will be:
// https://dashboard.svix.com/portal/{token}
// where {token} is a temporary access token generated by Svix
func (s *svixService) GetDashboardURL(ctx context.Context, applicationID string) (string, error) {
	req := &httpclient.Request{
		Method: http.MethodPost,
		URL:    fmt.Sprintf("%s/api/v1/app/%s/portal-token", s.config.Svix.BaseURL, applicationID),
		Headers: map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", s.config.Svix.AuthToken),
			"Content-Type":  "application/json",
		},
	}

	resp, err := s.client.Send(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to get Svix portal token: %w", err)
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", fmt.Errorf("failed to decode Svix response: %w", err)
	}

	return result.URL, nil
}

// SendMessage delivers an event to all configured endpoints for an application.
// The workflow is:
// 1. Convert payload to JSON
// 2. Send to Svix API
// 3. Svix handles delivery to all configured endpoints with:
//   - Automatic retries on failure
//   - Signature verification
//   - Rate limiting
//   - Delivery tracking
//
// API Endpoint Used:
// - POST {baseURL}/api/v1/app/{applicationID}/msg
//
// The eventType should follow a consistent pattern like:
// - resource.action (e.g., "invoice.created", "payment.failed")
// - namespace.resource.action (e.g., "billing.subscription.updated")
func (s *svixService) SendMessage(ctx context.Context, applicationID string, eventType string, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message payload: %w", err)
	}

	req := &httpclient.Request{
		Method: http.MethodPost,
		URL:    fmt.Sprintf("%s/api/v1/app/%s/msg", s.config.Svix.BaseURL, applicationID),
		Headers: map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", s.config.Svix.AuthToken),
			"Content-Type":  "application/json",
		},
		Body: []byte(fmt.Sprintf(`{"eventType":"%s","payload":%s}`, eventType, string(payloadBytes))),
	}

	resp, err := s.client.Send(ctx, req)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("application not found")
		}
		return fmt.Errorf("failed to send message to Svix: %w", err)
	}

	return nil
}
