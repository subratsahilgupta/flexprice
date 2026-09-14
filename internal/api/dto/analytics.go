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

// CreateViewRequest is the request for POST /v1/analytics/views.
type CreateViewRequest struct {
	Name       string                   `json:"name" validate:"required"`
	Definition analytics.ViewDefinition `json:"definition" validate:"required"`
}

func (r *CreateViewRequest) Validate() error {
	return validator.ValidateRequest(r)
}

// ViewQueryRequest is the request for POST /v1/analytics/views/{id}/query.
type ViewQueryRequest struct {
	Variables map[string]any `json:"variables"`
}

func (r *ViewQueryRequest) Validate() error {
	return validator.ValidateRequest(r)
}

// ViewResponse is the response for POST /v1/analytics/views. It exposes
// only the fields a client needs, unlike the domain analytics.View which
// embeds types.BaseModel (tenant_id, status, created_by, timestamps, ...).
type ViewResponse struct {
	ID         string                   `json:"id"`
	Name       string                   `json:"name"`
	Version    int                      `json:"version"`
	Definition analytics.ViewDefinition `json:"definition"`
}

// NewViewResponse maps a domain analytics.View onto its response DTO.
func NewViewResponse(v *analytics.View) *ViewResponse {
	if v == nil {
		return nil
	}
	resp := &ViewResponse{
		ID:      v.ID,
		Name:    v.Name,
		Version: v.Version,
	}
	if v.Definition != nil {
		resp.Definition = *v.Definition
	}
	return resp
}

// AnalyticsColumn describes one column of a shaped AnalyticsQueryResult.
type AnalyticsColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Role     string `json:"role"` // "dimension" | "metric"
	Currency string `json:"currency,omitempty"`
}

// AnalyticsQueryResult is the visualization-agnostic shape every analytics
// query renders into — the response body for POST /analytics/query and
// POST /analytics/views/{id}/query.
type AnalyticsQueryResult struct {
	Columns []*AnalyticsColumn `json:"columns"`
	Rows    [][]any            `json:"rows"`
	Meta    map[string]any     `json:"meta"`
}
