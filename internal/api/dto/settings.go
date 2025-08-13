package dto

import (
	"context"
	"errors"
	"strings"

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
	CreatedAt     string                 `json:"created_at"`
	UpdatedAt     string                 `json:"updated_at"`
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

	// Value is optional, so no validation needed for it
	return nil
}

// UpdateSettingRequest represents the request to update an existing setting
type UpdateSettingRequest struct {
	Value *map[string]interface{} `json:"value,omitempty"`
}

// Validate validates the UpdateSettingRequest
func (r *UpdateSettingRequest) Validate() error {
	// Value is optional for updates, so no validation needed for it
	return nil
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
		CreatedAt:     s.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
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

func (r *UpdateSettingRequest) ToSetting() (*settings.Setting, error) {
	setting := &settings.Setting{
		Value: *r.Value,
	}

	return setting, nil
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
