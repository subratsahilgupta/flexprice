package service

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/tenant"
)

type TenantService struct {
	repo tenant.Repository
}

func NewTenantService(repo tenant.Repository) *TenantService {
	return &TenantService{repo: repo}
}

func (s *TenantService) CreateTenant(ctx context.Context, req dto.CreateTenantRequest) (*dto.TenantResponse, error) {
	newTenant := &tenant.Tenant{
		Name: req.Name,
	}

	err := s.repo.Create(ctx, newTenant)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	return dto.NewTenantResponse(newTenant), nil
}

func (s *TenantService) GetTenantByID(ctx context.Context, id string) (*dto.TenantResponse, error) {
	t, err := s.repo.GetById(ctx, id)
	if err != nil {
		return nil, err
	}

	return dto.NewTenantResponse(t), nil
}
