package ent

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	entTenant "github.com/flexprice/flexprice/ent/tenant"
	domainTenant "github.com/flexprice/flexprice/internal/domain/tenant"
	ierr "github.com/flexprice/flexprice/internal/errors"
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

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "tenant", "create", map[string]interface{}{
		"tenant_id": tenant.ID,
		"name":      tenant.Name,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	_, err := client.Tenant.
		Create().
		SetID(tenant.ID).
		SetName(tenant.Name).
		SetStatus(string(tenant.Status)).
		SetCreatedAt(tenant.CreatedAt).
		SetUpdatedAt(tenant.UpdatedAt).
		SetBillingDetails(tenant.BillingDetails.ToSchema()).
		Save(ctx)

	if err != nil {
		SetSpanError(span, err)
		return ierr.WithError(err).
			WithHint("Failed to create tenant").
			WithReportableDetails(map[string]interface{}{
				"tenant_id": tenant.ID,
				"name":      tenant.Name,
			}).
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return nil
}

// GetByID retrieves a tenant by ID
func (r *tenantRepository) GetByID(ctx context.Context, id string) (*domainTenant.Tenant, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "tenant", "get_by_id", map[string]interface{}{
		"tenant_id": id,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	tenant, err := client.Tenant.
		Query().
		Where(
			entTenant.ID(id),
		).
		Only(ctx)

	if err != nil {
		SetSpanError(span, err)
		if ent.IsNotFound(err) {
			return nil, ierr.WithError(err).
				WithHint("Tenant not found").
				WithReportableDetails(map[string]interface{}{
					"tenant_id": id,
				}).
				Mark(ierr.ErrNotFound)
		}
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve tenant").
			WithReportableDetails(map[string]interface{}{
				"tenant_id": id,
			}).
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return domainTenant.FromEnt(tenant), nil
}

// List retrieves all tenants
func (r *tenantRepository) List(ctx context.Context) ([]*domainTenant.Tenant, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "tenant", "list", map[string]interface{}{})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	tenants, err := client.Tenant.
		Query().
		Order(ent.Desc(entTenant.FieldCreatedAt)).
		All(ctx)

	if err != nil {
		SetSpanError(span, err)
		return nil, ierr.WithError(err).
			WithHint("Failed to list tenants").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return domainTenant.FromEntList(tenants), nil
}

// Update implements tenant.Repository.
func (r *tenantRepository) Update(ctx context.Context, tenant *domainTenant.Tenant) error {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "tenant", "update", map[string]interface{}{
		"tenant_id": tenant.ID,
		"name":      tenant.Name,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)

	_, err := client.Tenant.
		UpdateOneID(tenant.ID).
		SetName(tenant.Name).
		SetBillingDetails(tenant.BillingDetails.ToSchema()).
		Save(ctx)

	if err != nil {
		SetSpanError(span, err)
		return ierr.WithError(err).
			WithHint("Failed to update tenant").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return nil
}
