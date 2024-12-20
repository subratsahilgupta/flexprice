package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/domain/tenant"
)

type TenantResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type CreateTenantRequest struct {
	Name string `json:"name" binding:"required"`
}

// NewTenantResponse converts a Tenant domain object into a TenantResponse DTO.
func NewTenantResponse(t *tenant.Tenant) *TenantResponse {
	return &TenantResponse{
		ID:        t.ID,
		Name:      t.Name,
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
	}
}
