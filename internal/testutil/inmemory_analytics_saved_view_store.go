package testutil

import (
	"context"

	domainAnalytics "github.com/flexprice/flexprice/internal/domain/analytics"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// InMemoryAnalyticsSavedViewStore implements analytics.Repository for testing.
type InMemoryAnalyticsSavedViewStore struct {
	*InMemoryStore[*domainAnalytics.SavedView]
}

// NewInMemoryAnalyticsSavedViewStore creates a new in-memory analytics saved view store.
func NewInMemoryAnalyticsSavedViewStore() *InMemoryAnalyticsSavedViewStore {
	return &InMemoryAnalyticsSavedViewStore{
		InMemoryStore: NewInMemoryStore[*domainAnalytics.SavedView](),
	}
}

func (s *InMemoryAnalyticsSavedViewStore) Create(ctx context.Context, v *domainAnalytics.SavedView) error {
	if v.TenantID == "" {
		v.TenantID = types.GetTenantID(ctx)
	}
	if v.Status == "" {
		v.Status = types.StatusPublished
	}
	return s.InMemoryStore.Create(ctx, v.ID, v)
}

func (s *InMemoryAnalyticsSavedViewStore) Get(ctx context.Context, id string) (*domainAnalytics.SavedView, error) {
	v, err := s.InMemoryStore.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Mirror the real ent repo's Get: scoped to tenant + status=published.
	// EnvironmentID isn't on the domain SavedView struct, so it can't be filtered here.
	if !CheckTenantFilter(ctx, v.TenantID) || v.Status != types.StatusPublished {
		return nil, ierr.NewError("item not found").
			WithHintf("Item with ID %s was not found", id).
			WithReportableDetails(map[string]any{
				"id": id,
			}).
			Mark(ierr.ErrNotFound)
	}

	return v, nil
}

func (s *InMemoryAnalyticsSavedViewStore) List(ctx context.Context) ([]*domainAnalytics.SavedView, error) {
	return s.InMemoryStore.List(ctx, nil, func(ctx context.Context, v *domainAnalytics.SavedView, _ interface{}) bool {
		return CheckTenantFilter(ctx, v.TenantID) && v.Status == types.StatusPublished
	}, nil)
}
