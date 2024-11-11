package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/domain"
	"github.com/flexprice/flexprice/internal/domain/meter"
)

// CreateMeterRequest represents the request payload for creating a meter
type CreateMeterRequest struct {
	Filters     []meter.Filter    `json:"filters"`
	Aggregation meter.Aggregation `json:"aggregation" binding:"required"`
	WindowSize  meter.WindowSize  `json:"window_size" binding:"required" example:"HOUR"`
}

// MeterResponse represents the meter response structure
type MeterResponse struct {
	ID          string            `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	TenantID    string            `json:"tenant_id" example:"tenant123"`
	Filters     []meter.Filter    `json:"filters"`
	Aggregation meter.Aggregation `json:"aggregation"`
	WindowSize  meter.WindowSize  `json:"window_size" example:"HOUR"`
	CreatedAt   time.Time         `json:"created_at" example:"2024-03-20T15:04:05Z"`
	UpdatedAt   time.Time         `json:"updated_at" example:"2024-03-20T15:04:05Z"`
	Status      string            `json:"status" example:"ACTIVE"`
}

// Convert domain Meter to MeterResponse
func ToMeterResponse(m *meter.Meter) *MeterResponse {
	return &MeterResponse{
		ID:          m.ID,
		TenantID:    m.TenantID,
		Filters:     m.Filters,
		Aggregation: m.Aggregation,
		WindowSize:  m.WindowSize,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		Status:      string(m.Status),
	}
}

// Convert CreateMeterRequest to domain Meter
func (r *CreateMeterRequest) ToMeter(createdBy string) *meter.Meter {
	if createdBy == "" {
		createdBy = "system"
	}

	m := meter.NewMeter("", createdBy)
	// TODO: Remove this once we have a way to set the tenant ID
	m.TenantID = "default"

	m.Filters = r.Filters
	m.Aggregation = r.Aggregation
	m.WindowSize = r.WindowSize
	m.Status = domain.StatusActive
	return m
}
