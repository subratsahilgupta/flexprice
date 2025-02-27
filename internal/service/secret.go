package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/secret"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// SecretService defines the interface for secret business logic
type SecretService interface {
	// API Key operations
	CreateAPIKey(ctx context.Context, req *dto.CreateAPIKeyRequest) (*secret.Secret, string, error)
	ListAPIKeys(ctx context.Context, filter *types.SecretFilter) (*dto.ListSecretsResponse, error)
	Delete(ctx context.Context, id string) error

	// Integration operations
	CreateIntegration(ctx context.Context, req *dto.CreateIntegrationRequest) (*secret.Secret, error)
	ListIntegrations(ctx context.Context, filter *types.SecretFilter) (*dto.ListSecretsResponse, error)

	// Verification operations
	VerifyAPIKey(ctx context.Context, apiKey string) (*secret.Secret, error)
	GetIntegrationCredentials(ctx context.Context, provider string) (map[string]string, error)
}

type secretService struct {
	repo              secret.Repository
	encryptionService security.EncryptionService
	config            *config.Configuration
	logger            *logger.Logger
}

// NewSecretService creates a new secret service
func NewSecretService(
	repo secret.Repository,
	config *config.Configuration,
	logger *logger.Logger,
) SecretService {
	encryptionService, err := security.NewEncryptionService(config, logger)
	if err != nil {
		logger.Fatalw("failed to create encryption service", "error", err)
	}

	return &secretService{
		repo:              repo,
		encryptionService: encryptionService,
		config:            config,
		logger:            logger,
	}
}

// Helper functions

// generatePrefix generates a prefix for an API key based on its type
func generatePrefix(keyType types.SecretType) string {
	switch keyType {
	case types.SecretTypePrivateKey:
		return "sk"
	case types.SecretTypePublishableKey:
		return "pk"
	default:
		return "key"
	}
}

// generateDisplayID generates a unique display ID for the secret
func generateDisplayID(apiKey string) string {
	return fmt.Sprintf("%s***%s", apiKey[:5], apiKey[len(apiKey)-2:])
}

// generateAPIKey generates a new API key
func generateAPIKey(prefix string) string {
	// Generate a ULID for the key value
	return types.GenerateUUIDWithPrefix(prefix)
}

func (s *secretService) CreateAPIKey(ctx context.Context, req *dto.CreateAPIKeyRequest) (*secret.Secret, string, error) {
	if err := req.Validate(); err != nil {
		return nil, "", err
	}

	// Generate API key
	prefix := generatePrefix(req.Type)
	apiKey := generateAPIKey(prefix)

	// Hash the entire API key for storage
	hashedKey := s.encryptionService.Hash(apiKey)

	// Set default permissions if none provided
	permissions := req.Permissions
	if len(permissions) == 0 {
		permissions = []string{"read", "write"}
	}

	// Create secret entity
	secretEntity := &secret.Secret{
		ID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SECRET),
		Name:          req.Name,
		Type:          req.Type,
		EnvironmentID: types.GetEnvironmentID(ctx),
		Provider:      types.SecretProviderFlexPrice,
		Value:         hashedKey,
		DisplayID:     generateDisplayID(apiKey),
		Permissions:   permissions,
		ExpiresAt:     req.ExpiresAt,
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}

	// Save to repository
	if err := s.repo.Create(ctx, secretEntity); err != nil {
		return nil, "", errors.Wrap(err, errors.ErrCodeInvalidOperation, "failed to create API key")
	}

	return secretEntity, apiKey, nil
}

func (s *secretService) ListAPIKeys(ctx context.Context, filter *types.SecretFilter) (*dto.ListSecretsResponse, error) {
	if filter == nil {
		filter = &types.SecretFilter{
			QueryFilter: types.NewDefaultQueryFilter(),
		}
	}

	// Set type filter for API keys
	filter.Type = lo.ToPtr(types.SecretTypePrivateKey)
	filter.Provider = lo.ToPtr(types.SecretProviderFlexPrice)
	filter.Status = lo.ToPtr(types.StatusPublished)

	secrets, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidOperation, "failed to list API keys")
	}

	count, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidOperation, "failed to count API keys")
	}

	return &dto.ListSecretsResponse{
		Items:      dto.ToSecretResponseList(secrets),
		Pagination: types.NewPaginationResponse(count, filter.GetLimit(), filter.GetOffset()),
	}, nil
}

func (s *secretService) CreateIntegration(ctx context.Context, req *dto.CreateIntegrationRequest) (*secret.Secret, error) {
	// Validate required fields
	if req.Name == "" {
		return nil, errors.New(errors.ErrCodeValidation, "validation failed: name is required")
	}
	if len(req.Credentials) == 0 {
		return nil, errors.New(errors.ErrCodeValidation, "validation failed: credentials are required")
	}
	if req.Provider == types.SecretProviderFlexPrice {
		return nil, errors.New(errors.ErrCodeValidation, "validation failed: invalid provider")
	}

	// Encrypt each credential
	encryptedCreds := make(map[string]string)
	for key, value := range req.Credentials {
		encrypted, err := s.encryptionService.Encrypt(value)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeSystemError, "failed to encrypt credentials")
		}
		encryptedCreds[key] = encrypted
	}

	// Generate a display ID for the integration
	displayID := types.GenerateUUIDWithPrefix("int")[:5]

	// Create secret entity
	secretEntity := &secret.Secret{
		ID:           types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SECRET),
		Name:         req.Name,
		Type:         types.SecretTypeIntegration,
		Provider:     req.Provider,
		Value:        "", // Empty for integrations
		DisplayID:    displayID,
		ProviderData: encryptedCreds,
		BaseModel:    types.GetDefaultBaseModel(ctx),
	}

	// Save to repository
	if err := s.repo.Create(ctx, secretEntity); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidOperation, "failed to create integration")
	}

	return secretEntity, nil
}

func (s *secretService) ListIntegrations(ctx context.Context, filter *types.SecretFilter) (*dto.ListSecretsResponse, error) {
	if filter == nil {
		filter = &types.SecretFilter{
			QueryFilter: types.NewDefaultQueryFilter(),
		}
	}

	// Set type filter for integrations
	filter.Type = lo.ToPtr(types.SecretTypeIntegration)
	filter.Status = lo.ToPtr(types.StatusPublished)

	secrets, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidOperation, "failed to list integrations")
	}

	count, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidOperation, "failed to count integrations")
	}

	return &dto.ListSecretsResponse{
		Items:      dto.ToSecretResponseList(secrets),
		Pagination: types.NewPaginationResponse(count, filter.GetLimit(), filter.GetOffset()),
	}, nil
}

func (s *secretService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return errors.Wrap(err, errors.ErrCodeInvalidOperation, "failed to delete secret")
	}
	return nil
}

func (s *secretService) VerifyAPIKey(ctx context.Context, apiKey string) (*secret.Secret, error) {
	if apiKey == "" {
		return nil, errors.New(errors.ErrCodeValidation, "validation failed: API key is required")
	}

	// Hash the entire API key for verification
	hashedKey := s.encryptionService.Hash(apiKey)

	// Get secret by value
	secretEntity, err := s.repo.GetAPIKeyByValue(ctx, hashedKey)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidOperation, "invalid API key")
	}

	// Check if expired
	if secretEntity.IsExpired() {
		return nil, errors.New(errors.ErrCodeValidation, "API key has expired")
	}

	// Check if secret is active
	if !secretEntity.IsActive() {
		return nil, errors.New(errors.ErrCodeValidation, "API key is not active")
	}

	// Check if secret is an API key
	if !secretEntity.IsAPIKey() {
		return nil, errors.New(errors.ErrCodeValidation, "invalid API key type")
	}

	// Check if the secret has expired
	if secretEntity.ExpiresAt != nil && secretEntity.ExpiresAt.Before(time.Now()) {
		return nil, errors.New(errors.ErrCodeInvalidOperation, "secret has expired")
	}

	// Update last used timestamp
	// TODO: Uncomment this when we have a way to efficiently update the last used timestamp
	// if err := s.repo.UpdateLastUsed(ctx, secretEntity.ID); err != nil {
	// 	s.logger.Warnw("failed to update last used timestamp", "error", err)
	// }

	return secretEntity, nil
}

func (s *secretService) GetIntegrationCredentials(ctx context.Context, provider string) (map[string]string, error) {
	filter := &types.SecretFilter{
		QueryFilter: types.NewNoLimitPublishedQueryFilter(),
		Type:        lo.ToPtr(types.SecretTypeIntegration),
		Provider:    lo.ToPtr(types.SecretProvider(provider)),
	}

	secrets, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidOperation, "failed to get integration credentials")
	}

	if len(secrets) == 0 {
		return nil, errors.Wrap(errors.ErrNotFound, errors.ErrCodeNotFound, fmt.Sprintf("%s integration not configured", provider))
	}

	if len(secrets) > 1 {
		return nil, errors.Wrap(errors.ErrInvalidOperation, "multiple integrations found for provider", provider)
	}

	// Use the first active integration
	secretEntity := secrets[0]

	// Decrypt each credential
	decryptedCreds := make(map[string]string)
	for key, encryptedValue := range secretEntity.ProviderData {
		decrypted, err := s.encryptionService.Decrypt(encryptedValue)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeSystemError, "failed to decrypt credentials")
		}
		decryptedCreds[key] = decrypted
	}

	return decryptedCreds, nil
}
