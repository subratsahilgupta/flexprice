package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/auth"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/tenant"
)

type TenantService interface {
	CreateTenant(ctx context.Context, req dto.CreateTenantRequest) (*dto.TenantResponse, error)
	GetTenantByID(ctx context.Context, id string) (*dto.TenantResponse, error)
	AssignTenantToUser(ctx context.Context, req dto.AssignTenantRequest) error
	GetAllTenants(ctx context.Context) ([]*dto.TenantResponse, error)
}

type tenantService struct {
	repo tenant.Repository
	cfg  *config.Configuration
}

func NewTenantService(repo tenant.Repository, cfg *config.Configuration) TenantService {
	return &tenantService{
		repo: repo,
		cfg:  cfg,
	}
}

func (s *tenantService) CreateTenant(ctx context.Context, req dto.CreateTenantRequest) (*dto.TenantResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	newTenant := req.ToTenant(ctx)

	if err := s.repo.Create(ctx, newTenant); err != nil {
		return nil, err
	}

	return dto.NewTenantResponse(newTenant), nil
}

func (s *tenantService) GetTenantByID(ctx context.Context, id string) (*dto.TenantResponse, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return dto.NewTenantResponse(t), nil
}

func (s *tenantService) AssignTenantToUser(ctx context.Context, req dto.AssignTenantRequest) error {
	if err := req.Validate(ctx); err != nil {
		return err
	}

	// Verify tenant exists
	_, err := s.GetTenantByID(ctx, req.TenantID)
	if err != nil {
		return err
	}

	authProvider := auth.NewProvider(s.cfg)

	// Assign tenant to user using auth provider
	if err := authProvider.AssignUserToTenant(ctx, req.UserID, req.TenantID); err != nil {
		return err
	}

	return nil
}

func (s *tenantService) GetAllTenants(ctx context.Context) ([]*dto.TenantResponse, error) {
	tenants, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	tenantResponses := make([]*dto.TenantResponse, 0, len(tenants))
	for _, t := range tenants {
		tenantResponses = append(tenantResponses, dto.NewTenantResponse(t))
	}

	return tenantResponses, nil
}
