package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/flexprice/flexprice/internal/cache"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

const (
	// Cache keys
	cacheKeyGracePeriod = "subscription:lifecycle:grace_period:%s:%s"
	cacheKeyDueDate     = "subscription:lifecycle:due_date:%s:%s"

	// Cache TTL
	configCacheTTL = 1 * time.Hour
)

// SubscriptionLifecycleService handles subscription lifecycle operations
type SubscriptionLifecycleService struct {
	configRepo subscription.SubscriptionLifecycleConfigRepository
	cache      cache.Cache
}

// NewSubscriptionLifecycleService creates a new subscription lifecycle service
func NewSubscriptionLifecycleService(
	configRepo subscription.SubscriptionLifecycleConfigRepository,
	cache cache.Cache,
) *SubscriptionLifecycleService {
	return &SubscriptionLifecycleService{
		configRepo: configRepo,
		cache:      cache,
	}
}

// GetDefaultGracePeriod returns the default grace period in days
func (s *SubscriptionLifecycleService) GetDefaultGracePeriod(ctx context.Context, tenantID, environmentID string) (int, error) {
	// Try cache first
	cacheKey := getCacheKey(cacheKeyGracePeriod, tenantID, environmentID)
	val, found := s.cache.Get(ctx, cacheKey)
	if found && val != nil {
		if strVal, ok := val.(string); ok {
			if days, err := strconv.Atoi(strVal); err == nil {
				return days, nil
			}
		}
	}

	// Get from database
	config, err := s.configRepo.GetConfig(ctx, tenantID, environmentID, subscription.ConfigKeyDefaultGracePeriod)
	if err != nil {
		if errors.IsNotFound(err) {
			return subscription.DefaultGracePeriodDays, nil
		}
		return 0, err
	}

	days, err := strconv.Atoi(config.Value)
	if err != nil {
		return 0, errors.NewError("invalid grace period value in config").
			WithHint("Config value must be a valid integer").
			Mark(errors.ErrValidation)
	}

	// Cache the result
	s.cache.Set(ctx, cacheKey, config.Value, configCacheTTL)

	return days, nil
}

// SetDefaultGracePeriod sets the default grace period in days
func (s *SubscriptionLifecycleService) SetDefaultGracePeriod(ctx context.Context, tenantID, environmentID string, days int, userID string) error {
	if days < 0 {
		return errors.NewError("grace period days cannot be negative").
			WithHint("Grace period must be 0 or greater").
			Mark(errors.ErrValidation)
	}

	// Get existing config if any
	var previousValue string
	existingConfig, err := s.configRepo.GetConfig(ctx, tenantID, environmentID, subscription.ConfigKeyDefaultGracePeriod)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if existingConfig != nil {
		previousValue = existingConfig.Value
	}

	// Create or update config
	config := &subscription.SubscriptionLifecycleConfig{
		TenantID:      tenantID,
		EnvironmentID: environmentID,
		Key:           subscription.ConfigKeyDefaultGracePeriod,
		Value:         strconv.Itoa(days),
		UpdatedAt:     time.Now(),
		UpdatedBy:     userID,
		Status:        string(types.StatusPublished),
	}

	if existingConfig == nil {
		config.ID = fmt.Sprintf("%s_%s", types.UUID_PREFIX_SUBSCRIPTION, types.GenerateUUID())
		config.CreatedAt = config.UpdatedAt
		config.CreatedBy = userID
	} else {
		config.ID = existingConfig.ID
		config.CreatedAt = existingConfig.CreatedAt
		config.CreatedBy = existingConfig.CreatedBy
	}

	if err := s.configRepo.SetConfig(ctx, config); err != nil {
		return err
	}

	// Create audit log
	audit := &subscription.SubscriptionLifecycleConfigAudit{
		ID:            fmt.Sprintf("%s_%s", types.UUID_PREFIX_SUBSCRIPTION, types.GenerateUUID()),
		TenantID:      tenantID,
		EnvironmentID: environmentID,
		ConfigID:      config.ID,
		Key:           config.Key,
		PreviousValue: previousValue,
		NewValue:      config.Value,
		ChangedAt:     time.Now(),
		ChangedBy:     userID,
	}

	if err := s.configRepo.CreateConfigAudit(ctx, audit); err != nil {
		return err
	}

	// Invalidate cache
	cacheKey := getCacheKey(cacheKeyGracePeriod, tenantID, environmentID)
	s.cache.Delete(ctx, cacheKey)

	return nil
}

// GetDefaultDueDate returns the default due date in days
func (s *SubscriptionLifecycleService) GetDefaultDueDate(ctx context.Context, tenantID, environmentID string) (int, error) {
	// Try cache first
	cacheKey := getCacheKey(cacheKeyDueDate, tenantID, environmentID)
	val, found := s.cache.Get(ctx, cacheKey)
	if found && val != nil {
		if strVal, ok := val.(string); ok {
			if days, err := strconv.Atoi(strVal); err == nil {
				return days, nil
			}
		}
	}

	// Get from database
	config, err := s.configRepo.GetConfig(ctx, tenantID, environmentID, subscription.ConfigKeyDefaultDueDate)
	if err != nil {
		if errors.IsNotFound(err) {
			return subscription.DefaultDueDateDays, nil
		}
		return 0, err
	}

	days, err := strconv.Atoi(config.Value)
	if err != nil {
		return 0, errors.NewError("invalid due date value in config").
			WithHint("Config value must be a valid integer").
			Mark(errors.ErrValidation)
	}

	// Cache the result
	s.cache.Set(ctx, cacheKey, config.Value, configCacheTTL)

	return days, nil
}

// SetDefaultDueDate sets the default due date in days
func (s *SubscriptionLifecycleService) SetDefaultDueDate(ctx context.Context, tenantID, environmentID string, days int, userID string) error {
	if days < 0 {
		return errors.NewError("due date days cannot be negative").
			WithHint("Due date must be 0 or greater").
			Mark(errors.ErrValidation)
	}

	// Get existing config if any
	var previousValue string
	existingConfig, err := s.configRepo.GetConfig(ctx, tenantID, environmentID, subscription.ConfigKeyDefaultDueDate)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if existingConfig != nil {
		previousValue = existingConfig.Value
	}

	// Create or update config
	config := &subscription.SubscriptionLifecycleConfig{
		TenantID:      tenantID,
		EnvironmentID: environmentID,
		Key:           subscription.ConfigKeyDefaultDueDate,
		Value:         strconv.Itoa(days),
		UpdatedAt:     time.Now(),
		UpdatedBy:     userID,
		Status:        string(types.StatusPublished),
	}

	if existingConfig == nil {
		config.ID = fmt.Sprintf("%s_%s", types.UUID_PREFIX_SUBSCRIPTION, types.GenerateUUID())
		config.CreatedAt = config.UpdatedAt
		config.CreatedBy = userID
	} else {
		config.ID = existingConfig.ID
		config.CreatedAt = existingConfig.CreatedAt
		config.CreatedBy = existingConfig.CreatedBy
	}

	if err := s.configRepo.SetConfig(ctx, config); err != nil {
		return err
	}

	// Create audit log
	audit := &subscription.SubscriptionLifecycleConfigAudit{
		ID:            fmt.Sprintf("%s_%s", types.UUID_PREFIX_SUBSCRIPTION, types.GenerateUUID()),
		TenantID:      tenantID,
		EnvironmentID: environmentID,
		ConfigID:      config.ID,
		Key:           config.Key,
		PreviousValue: previousValue,
		NewValue:      config.Value,
		ChangedAt:     time.Now(),
		ChangedBy:     userID,
	}

	if err := s.configRepo.CreateConfigAudit(ctx, audit); err != nil {
		return err
	}

	// Invalidate cache
	cacheKey := getCacheKey(cacheKeyDueDate, tenantID, environmentID)
	s.cache.Delete(ctx, cacheKey)

	return nil
}

// Helper function to generate cache keys
func getCacheKey(format, tenantID, environmentID string) string {
	return fmt.Sprintf(format, tenantID, environmentID)
}
