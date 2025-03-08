package dto

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/tenant"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
)

type CreateTenantRequest struct {
	Name string `json:"name" validate:"required"`
}

type TenantResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type AssignTenantRequest struct {
	UserID   string `json:"user_id" validate:"required,uuid"`
	TenantID string `json:"tenant_id" validate:"required,uuid"`
}

func (r *CreateTenantRequest) Validate() error {
	return validator.ValidateRequest(r)
}

func (r *CreateTenantRequest) ToTenant(ctx context.Context) *tenant.Tenant {
	return &tenant.Tenant{
		ID:        types.GenerateUUIDWithPrefix(types.UUID_PREFIX_TENANT),
		Name:      r.Name,
		Status:    types.StatusPublished,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (r *AssignTenantRequest) Validate(ctx context.Context) error {
	return validator.ValidateRequest(r)
}

func NewTenantResponse(t *tenant.Tenant) *TenantResponse {
	return &TenantResponse{
		ID:        t.ID,
		Name:      t.Name,
		Status:    string(t.Status),
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
	}
}
