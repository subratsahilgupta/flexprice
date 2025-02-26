package ent

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/ent"
	entTenant "github.com/flexprice/flexprice/ent/tenant"
	domainTenant "github.com/flexprice/flexprice/internal/domain/tenant"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
)

type tenantRepository struct {
	client postgres.IClient
	logger *logger.Logger
}

// NewTenantRepository creates a new tenant repository
func NewTenantRepository(client postgres.IClient, logger *logger.Logger) domainTenant.Repository {
	return &tenantRepository{
		client: client,
		logger: logger,
	}
}

// Create creates a new tenant
func (r *tenantRepository) Create(ctx context.Context, tenant *domainTenant.Tenant) error {
	r.logger.Debugw("creating tenant", "tenant_id", tenant.ID, "name", tenant.Name)

	client := r.client.Querier(ctx)
	_, err := client.Tenant.
		Create().
		SetID(tenant.ID).
		SetName(tenant.Name).
		SetStatus(string(tenant.Status)).
		SetCreatedAt(tenant.CreatedAt).
		SetUpdatedAt(tenant.UpdatedAt).
		Save(ctx)

	if err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	return nil
}

// GetByID retrieves a tenant by ID
func (r *tenantRepository) GetByID(ctx context.Context, id string) (*domainTenant.Tenant, error) {
	client := r.client.Querier(ctx)
	tenant, err := client.Tenant.
		Query().
		Where(
			entTenant.ID(id),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("tenant not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return domainTenant.FromEnt(tenant), nil
}

// List retrieves all tenants
func (r *tenantRepository) List(ctx context.Context) ([]*domainTenant.Tenant, error) {
	client := r.client.Querier(ctx)
	tenants, err := client.Tenant.
		Query().
		Order(ent.Desc(entTenant.FieldCreatedAt)).
		All(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	return domainTenant.FromEntList(tenants), nil
}
