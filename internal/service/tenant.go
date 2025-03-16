package service

import (
	"context"
	"time"

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
	UpdateTenant(ctx context.Context, id string, req dto.UpdateTenantRequest) (*dto.TenantResponse, error)
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

func (s *tenantService) UpdateTenant(ctx context.Context, id string, req dto.UpdateTenantRequest) (*dto.TenantResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get the existing tenant
	existingTenant, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.BillingDetails != (dto.TenantBillingDetails{}) {
		// Convert from DTO to domain
		billingDetails := tenant.BillingDetails{
			Email:     req.BillingDetails.Email,
			HelpEmail: req.BillingDetails.HelpEmail,
			Phone:     req.BillingDetails.Phone,
			Address: tenant.Address{
				Line1:      req.BillingDetails.Address.Line1,
				Line2:      req.BillingDetails.Address.Line2,
				City:       req.BillingDetails.Address.City,
				State:      req.BillingDetails.Address.State,
				PostalCode: req.BillingDetails.Address.PostalCode,
				Country:    req.BillingDetails.Address.Country,
			},
		}
		existingTenant.BillingDetails = billingDetails
	}

	// Update the timestamp
	existingTenant.UpdatedAt = time.Now()

	// Save the updated tenant
	if err := s.repo.Update(ctx, existingTenant); err != nil {
		return nil, err
	}

	return dto.NewTenantResponse(existingTenant), nil
}
