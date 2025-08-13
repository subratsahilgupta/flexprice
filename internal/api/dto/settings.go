package dto

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/domain/settings"
	"github.com/flexprice/flexprice/internal/types"
)

// SettingResponse represents a setting in API responses
type SettingResponse struct {
	ID            string                 `json:"id"`
	Key           string                 `json:"key"`
	Value         map[string]interface{} `json:"value"`
	EnvironmentID string                 `json:"environment_id"`
	TenantID      string                 `json:"tenant_id"`
	Status        string                 `json:"status"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	CreatedBy     string                 `json:"created_by,omitempty"`
	UpdatedBy     string                 `json:"updated_by,omitempty"`
}

// CreateSettingRequest represents the request to create a new setting
type CreateSettingRequest struct {
	Key   string                 `json:"key" validate:"required,min=1,max=255"`
	Value map[string]interface{} `json:"value,omitempty"`
}

// Validate validates the CreateSettingRequest
func (r *CreateSettingRequest) Validate() error {
	if strings.TrimSpace(r.Key) == "" {
		return errors.New("key is required and cannot be empty")
	}

	if len(r.Key) > 255 {
		return errors.New("key cannot exceed 255 characters")
	}

	// Only validate value if it's provided
	if r.Value != nil {
		if err := r.ValidateValueByKey(); err != nil {
			return err
		}
	}

	return nil
}

// UpdateSettingRequest represents the request to update an existing setting
type UpdateSettingRequest struct {
	Value *map[string]interface{} `json:"value,omitempty"`
}

// Validate validates the UpdateSettingRequest
func (r *UpdateSettingRequest) Validate() error {
	if r.Value == nil {
		return errors.New("value is required for updates")
	}

	return nil
}

// ValidateValueByKey validates the value field based on the specific setting key
func (r *UpdateSettingRequest) ValidateValueByKey(key string) error {
	if r.Value == nil {
		return errors.New("value cannot be nil")
	}

	return validateSettingValue(key, *r.Value)
}

// SettingFromDomain converts a domain setting to DTO
func SettingFromDomain(s *settings.Setting) *SettingResponse {
	if s == nil {
		return nil
	}

	return &SettingResponse{
		ID:            s.ID,
		Key:           s.Key,
		Value:         s.Value,
		EnvironmentID: s.EnvironmentID,
		TenantID:      s.TenantID,
		Status:        string(s.Status),
		CreatedAt:     s.BaseModel.CreatedAt,
		UpdatedAt:     s.BaseModel.UpdatedAt,
		CreatedBy:     s.CreatedBy,
		UpdatedBy:     s.UpdatedBy,
	}
}

// SettingsFromDomain converts a list of domain settings to DTOs
func SettingsFromDomain(settingsList []*settings.Setting) []*SettingResponse {
	if settingsList == nil {
		return nil
	}

	result := make([]*SettingResponse, len(settingsList))
	for i, s := range settingsList {
		result[i] = SettingFromDomain(s)
	}

	return result
}

func (r *CreateSettingRequest) ToSetting(ctx context.Context) *settings.Setting {
	return &settings.Setting{
		ID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SETTING),
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
		Key:           r.Key,
		Value:         r.Value,
	}
}

// ValidateValueByKey validates the value field based on the specific setting key
func (r *CreateSettingRequest) ValidateValueByKey() error {
	if r.Value == nil {
		return errors.New("value cannot be nil")
	}
	return validateSettingValue(r.Key, r.Value)
}

// validateSettingValue validates a setting value based on its key
func validateSettingValue(key string, value map[string]interface{}) error {
	switch types.SettingKey(key) {
	case types.SettingKeyInvoiceConfig:
		return validateInvoiceConfig(value)
	default:
		// For unknown keys, just validate that it's a valid JSON object
		if value == nil {
			return errors.New("value cannot be nil")
		}
		return nil
	}
}

// validateInvoiceConfig validates invoice configuration settings
func validateInvoiceConfig(value map[string]interface{}) error {
	if value == nil {
		return errors.New("invoice_config value cannot be nil")
	}

	// Validate prefix
	prefixRaw, exists := value["prefix"]
	if !exists {
		return errors.New("invoice_config: 'prefix' is required")
	}
	prefix, ok := prefixRaw.(string)
	if !ok {
		return fmt.Errorf("invoice_config: 'prefix' must be a string, got %T", prefixRaw)
	}
	if strings.TrimSpace(prefix) == "" {
		return errors.New("invoice_config: 'prefix' cannot be empty")
	}

	// Validate format
	formatRaw, exists := value["format"]
	if !exists {
		return errors.New("invoice_config: 'format' is required")
	}
	_, ok = formatRaw.(string)
	if !ok {
		return fmt.Errorf("invoice_config: 'format' must be a string, got %T", formatRaw)
	}

	// Validate start_sequence
	startSeqRaw, exists := value["start_sequence"]
	if !exists {
		return errors.New("invoice_config: 'start_sequence' is required")
	}

	// Convert and validate start_sequence
	var startSeq int
	switch v := startSeqRaw.(type) {
	case int:
		startSeq = v
	case float64:
		// Check if it's a whole number
		if v != float64(int(v)) {
			return errors.New("invoice_config: 'start_sequence' must be a whole number")
		}
		startSeq = int(v)
	default:
		return fmt.Errorf("invoice_config: 'start_sequence' must be an integer, got %T", startSeqRaw)
	}

	// Validate range
	if startSeq < 1 {
		return errors.New("invoice_config: 'start_sequence' must be greater than 0")
	}

	return nil
}

// GetTypedValue safely extracts and converts a typed value from the settings map
func GetTypedValue[T any](value map[string]interface{}, key string) (T, error) {
	var zero T

	raw, exists := value[key]
	if !exists {
		return zero, fmt.Errorf("key '%s' not found", key)
	}

	typed, ok := raw.(T)
	if !ok {
		return zero, fmt.Errorf("key '%s' expected type %T but got %T", key, zero, raw)
	}

	return typed, nil
}

// GetInvoiceConfigSafely safely extracts invoice configuration from settings value
func GetInvoiceConfigSafely(value map[string]interface{}) (*types.InvoiceConfig, error) {
	if err := validateInvoiceConfig(value); err != nil {
		return nil, err
	}

	// Extract with proper error handling
	prefix, err := GetTypedValue[string](value, "prefix")
	if err != nil {
		return nil, fmt.Errorf("failed to extract prefix: %w", err)
	}

	format, err := GetTypedValue[string](value, "format")
	if err != nil {
		return nil, fmt.Errorf("failed to extract format: %w", err)
	}

	// Handle start_sequence with proper conversion
	var startSequence int
	startSeqRaw := value["start_sequence"]
	switch v := startSeqRaw.(type) {
	case int:
		startSequence = v
	case float64:
		if v != float64(int(v)) {
			return nil, errors.New("start_sequence must be a whole number")
		}
		startSequence = int(v)
	default:
		return nil, fmt.Errorf("start_sequence must be an integer, got %T", startSeqRaw)
	}

	return &types.InvoiceConfig{
		InvoiceNumberPrefix:        prefix,
		InvoiceNumberFormat:        format,
		InvoiceNumberStartSequence: startSequence,
	}, nil
}
