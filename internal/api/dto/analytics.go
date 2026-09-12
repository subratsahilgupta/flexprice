package dto

import (
	"github.com/flexprice/flexprice/internal/domain/analytics"
	"github.com/flexprice/flexprice/internal/validator"
)

// AnalyticsQueryRequest is the request for POST /v1/analytics/query (ad-hoc, unsaved view).
type AnalyticsQueryRequest struct {
	Definition analytics.ViewDefinition `json:"definition" validate:"required"`
	Variables  map[string]any           `json:"variables"`
}

func (r *AnalyticsQueryRequest) Validate() error {
	return validator.ValidateRequest(r)
}

// CreateSavedViewRequest is the request for POST /v1/analytics/views.
type CreateSavedViewRequest struct {
	Name       string                   `json:"name" validate:"required"`
	Definition analytics.ViewDefinition `json:"definition" validate:"required"`
}

func (r *CreateSavedViewRequest) Validate() error {
	return validator.ValidateRequest(r)
}

// SavedViewQueryRequest is the request for POST /v1/analytics/views/{id}/query.
type SavedViewQueryRequest struct {
	Variables map[string]any `json:"variables"`
}

func (r *SavedViewQueryRequest) Validate() error {
	return validator.ValidateRequest(r)
}
