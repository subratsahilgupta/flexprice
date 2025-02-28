package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/domain/secret"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/go-playground/validator/v10"
	"github.com/samber/lo"
)

// CreateAPIKeyRequest represents the request to create a new API key
type CreateAPIKeyRequest struct {
	Name        string           `json:"name" binding:"required" validate:"required"`
	Type        types.SecretType `json:"type" binding:"required,oneof=private_key publishable_key" validate:"required,oneof=private_key publishable_key"`
	Permissions []string         `json:"permissions"`
	ExpiresAt   *time.Time       `json:"expires_at,omitempty"`
}

func (r *CreateAPIKeyRequest) Validate() error {
	err := validator.New().Struct(r)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeValidation, "failed to validate create api key request")
	}

	if err := r.Type.Validate(); err != nil {
		return errors.Wrap(err, errors.ErrCodeValidation, "invalid secret type")
	}

	allowedSecretTypes := []types.SecretType{types.SecretTypePrivateKey, types.SecretTypePublishableKey}
	if !lo.Contains(allowedSecretTypes, r.Type) {
		return errors.Wrap(errors.ErrValidation, errors.ErrCodeValidation, "invalid secret type")
	}

	return nil
}

// CreateIntegrationRequest represents the request to create/update an integration
type CreateIntegrationRequest struct {
	Name        string               `json:"name" binding:"required"`
	Provider    types.SecretProvider `json:"provider" binding:"required"`
	Credentials map[string]string    `json:"credentials" binding:"required"`
}

func (r *CreateIntegrationRequest) Validate() error {
	err := validator.New().Struct(r)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeValidation, "failed to validate create integration request")
	}

	if r.Provider == types.SecretProviderFlexPrice {
		return errors.Wrap(errors.ErrValidation, errors.ErrCodeValidation, "flexprice provider is not allowed to be used for integrations")
	}

	return nil
}

// SecretResponse represents a secret in responses
type SecretResponse struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Type        types.SecretType     `json:"type"`
	Provider    types.SecretProvider `json:"provider"`
	DisplayID   string               `json:"display_id"`
	Permissions []string             `json:"permissions"`
	ExpiresAt   *time.Time           `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time           `json:"last_used_at,omitempty"`
	Status      types.Status         `json:"status"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// CreateAPIKeyResponse represents the response when creating a new API key
type CreateAPIKeyResponse struct {
	Secret SecretResponse `json:"secret"`
	APIKey string         `json:"api_key,omitempty"`
}

// ListSecretsResponse represents the response for listing secrets
type ListSecretsResponse = types.ListResponse[*SecretResponse]

// ToSecretResponse converts a domain Secret to a SecretResponse
func ToSecretResponse(s *secret.Secret) *SecretResponse {
	if s == nil {
		return nil
	}

	return &SecretResponse{
		ID:          s.ID,
		Name:        s.Name,
		Type:        s.Type,
		Provider:    s.Provider,
		DisplayID:   s.DisplayID,
		Permissions: s.Permissions,
		ExpiresAt:   s.ExpiresAt,
		LastUsedAt:  s.LastUsedAt,
		Status:      s.Status,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

// ToSecretResponseList converts a list of domain Secrets to SecretResponses
func ToSecretResponseList(secrets []*secret.Secret) []*SecretResponse {
	responses := make([]*SecretResponse, len(secrets))
	for i, s := range secrets {
		responses[i] = ToSecretResponse(s)
	}
	return responses
}

// LinkedIntegrationsResponse represents the response for listing linked integrations
type LinkedIntegrationsResponse struct {
	Providers []string `json:"providers"`
}
