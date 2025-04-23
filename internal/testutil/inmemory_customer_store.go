package testutil

import (
	"context"
	"strings"

	"github.com/flexprice/flexprice/internal/domain/customer"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// InMemoryCustomerStore implements customer.Repository
type InMemoryCustomerStore struct {
	*InMemoryStore[*customer.Customer]
}

// NewInMemoryCustomerStore creates a new in-memory customer store
func NewInMemoryCustomerStore() *InMemoryCustomerStore {
	return &InMemoryCustomerStore{
		InMemoryStore: NewInMemoryStore[*customer.Customer](),
	}
}

// Helper to copy customer
func copyCustomer(c *customer.Customer) *customer.Customer {
	if c == nil {
		return nil
	}

	// Deep copy of customer
	c = &customer.Customer{
		ID:                c.ID,
		ExternalID:        c.ExternalID,
		Name:              c.Name,
		Email:             c.Email,
		AddressLine1:      c.AddressLine1,
		AddressLine2:      c.AddressLine2,
		AddressCity:       c.AddressCity,
		AddressState:      c.AddressState,
		AddressPostalCode: c.AddressPostalCode,
		AddressCountry:    c.AddressCountry,
		Metadata:          lo.Assign(map[string]string{}, c.Metadata),
		EnvironmentID:     c.EnvironmentID,
		BaseModel: types.BaseModel{
			TenantID:  c.TenantID,
			Status:    c.Status,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			CreatedBy: c.CreatedBy,
			UpdatedBy: c.UpdatedBy,
		},
	}

	return c
}

func (s *InMemoryCustomerStore) Create(ctx context.Context, c *customer.Customer) error {
	// Set environment ID from context if not already set
	if c.EnvironmentID == "" {
		c.EnvironmentID = types.GetEnvironmentID(ctx)
	}
	return s.InMemoryStore.Create(ctx, c.ID, copyCustomer(c))
}

func (s *InMemoryCustomerStore) Get(ctx context.Context, id string) (*customer.Customer, error) {
	c, err := s.InMemoryStore.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return copyCustomer(c), nil
}

func (s *InMemoryCustomerStore) GetByLookupKey(ctx context.Context, lookupKey string) (*customer.Customer, error) {
	// Create a filter function that matches by external_id, tenant_id, and environment_id
	filterFn := func(ctx context.Context, c *customer.Customer, _ interface{}) bool {
		return c.ExternalID == lookupKey &&
			c.TenantID == types.GetTenantID(ctx) &&
			CheckEnvironmentFilter(ctx, c.EnvironmentID)
	}

	// List all customers with our filter
	customers, err := s.InMemoryStore.List(ctx, nil, filterFn, nil)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list customers").
			Mark(ierr.ErrDatabase)
	}

	if len(customers) == 0 {
		return nil, ierr.NewError("customer not found").
			WithHintf("Customer with lookup key %s was not found", lookupKey).
			WithReportableDetails(map[string]any{
				"lookup_key": lookupKey,
			}).
			Mark(ierr.ErrNotFound)
	}

	return copyCustomer(customers[0]), nil
}

func (s *InMemoryCustomerStore) List(ctx context.Context, filter *types.CustomerFilter) ([]*customer.Customer, error) {
	items, err := s.InMemoryStore.List(ctx, filter, customerFilterFn, customerSortFn)
	if err != nil {
		return nil, err
	}

	return lo.Map(items, func(c *customer.Customer, _ int) *customer.Customer {
		return copyCustomer(c)
	}), nil
}

func (s *InMemoryCustomerStore) Count(ctx context.Context, filter *types.CustomerFilter) (int, error) {
	return s.InMemoryStore.Count(ctx, filter, customerFilterFn)
}

func (s *InMemoryCustomerStore) ListAll(ctx context.Context, filter *types.CustomerFilter) ([]*customer.Customer, error) {
	f := *filter
	f.QueryFilter = types.NewNoLimitQueryFilter()
	return s.List(ctx, &f)
}

func (s *InMemoryCustomerStore) Update(ctx context.Context, c *customer.Customer) error {
	return s.InMemoryStore.Update(ctx, c.ID, copyCustomer(c))
}

func (s *InMemoryCustomerStore) Delete(ctx context.Context, id string) error {
	return s.InMemoryStore.Delete(ctx, id)
}

// customerFilterFn implements filtering logic for customers
func customerFilterFn(ctx context.Context, c *customer.Customer, filter interface{}) bool {
	f, ok := filter.(*types.CustomerFilter)
	if !ok {
		return false
	}

	// Apply tenant filter
	tenantID := types.GetTenantID(ctx)
	if tenantID != "" && c.TenantID != tenantID {
		return false
	}

	// Apply environment filter
	if !CheckEnvironmentFilter(ctx, c.EnvironmentID) {
		return false
	}

	// Apply external ID filter
	if f.ExternalID != "" && c.ExternalID != f.ExternalID {
		return false
	}

	// Apply email filter
	if f.Email != "" && !strings.EqualFold(c.Email, f.Email) {
		return false
	}

	// Apply customer ID filter
	if len(f.CustomerIDs) > 0 && !lo.Contains(f.CustomerIDs, c.ID) {
		return false
	}

	// Apply time range filter if present
	if f.TimeRangeFilter != nil {
		if f.StartTime != nil && c.CreatedAt.Before(*f.StartTime) {
			return false
		}
		if f.EndTime != nil && c.CreatedAt.After(*f.EndTime) {
			return false
		}
	}

	return true
}

// customerSortFn implements sorting logic for customers
func customerSortFn(i, j *customer.Customer) bool {
	// Default sort by created_at desc
	return i.CreatedAt.After(j.CreatedAt)
}

func (s *InMemoryCustomerStore) CountByFilter(ctx context.Context, filter *types.CustomerSearchFilter) (int, error) {
	// Create a custom filter function for CustomerSearchFilter
	filterFn := func(ctx context.Context, c *customer.Customer, _ interface{}) bool {
		// Check tenant ID
		if tenantID, ok := ctx.Value(types.CtxTenantID).(string); ok {
			if c.TenantID != tenantID {
				return false
			}
		}

		// Apply environment filter
		if !CheckEnvironmentFilter(ctx, c.EnvironmentID) {
			return false
		}

		// Filter by customer ID (partial match)
		if filter != nil && filter.CustomerID != nil {
			customerIDStr := *filter.CustomerID
			if !strings.Contains(strings.ToLower(c.ID), strings.ToLower(customerIDStr)) {
				return false
			}
		}

		// Filter by external ID (case-insensitive partial match)
		if filter != nil && filter.ExternalID != nil {
			externalIDStr := *filter.ExternalID
			if !strings.Contains(strings.ToLower(c.ExternalID), strings.ToLower(externalIDStr)) {
				return false
			}
		}

		return true
	}

	// Use the custom filter function for counting
	return s.InMemoryStore.Count(ctx, filter, filterFn)
}

func (s *InMemoryCustomerStore) ListByFilter(ctx context.Context, filter *types.CustomerSearchFilter) ([]*customer.Customer, error) {
	// Use the existing List method with a custom filter function
	filterFn := func(ctx context.Context, c *customer.Customer, _ interface{}) bool {
		// Apply search filter logic
		if filter == nil {
			return true
		}

		// Check tenant ID
		if tenantID, ok := ctx.Value(types.CtxTenantID).(string); ok {
			if c.TenantID != tenantID {
				return false
			}
		}

		// Apply environment filter
		if !CheckEnvironmentFilter(ctx, c.EnvironmentID) {
			return false
		}

		// Filter by customer ID
		if filter.CustomerID != nil && !strings.Contains(strings.ToLower(c.ID), strings.ToLower(*filter.CustomerID)) {
			return false
		}

		// Filter by external ID
		if filter.ExternalID != nil && !strings.Contains(strings.ToLower(c.ExternalID), strings.ToLower(*filter.ExternalID)) {
			return false
		}

		return true
	}

	// Prepare the query filter
	queryFilter := types.NewDefaultQueryFilter()
	if filter != nil {
		if filter.Limit != nil {
			queryFilter.Limit = filter.Limit
		}
		if filter.Offset != nil {
			queryFilter.Offset = filter.Offset
		}
	}

	// Use the InMemoryStore's List method with our custom filter
	customers, err := s.InMemoryStore.List(ctx, queryFilter, filterFn, customerSortFn)
	if err != nil {
		return nil, err
	}

	// Deep copy customers to prevent modification
	return lo.Map(customers, func(c *customer.Customer, _ int) *customer.Customer {
		return copyCustomer(c)
	}), nil
}
