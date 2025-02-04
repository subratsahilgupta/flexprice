package testutil

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/domain/feature"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// InMemoryFeatureStore implements feature.Repository
type InMemoryFeatureStore struct {
	*InMemoryStore[*feature.Feature]
}

func NewInMemoryFeatureStore() *InMemoryFeatureStore {
	return &InMemoryFeatureStore{
		InMemoryStore: NewInMemoryStore[*feature.Feature](),
	}
}

// featureFilterFn implements filtering logic for features
func featureFilterFn(ctx context.Context, f *feature.Feature, filter interface{}) bool {
	if f == nil {
		return false
	}

	featureFilter, ok := filter.(*types.FeatureFilter)
	if !ok {
		return true // No filter applied
	}

	// Check tenant ID
	if tenantID, ok := ctx.Value(types.CtxTenantID).(string); ok {
		if f.TenantID != tenantID {
			return false
		}
	}

	// Filter by feature IDs
	if len(featureFilter.FeatureIDs) > 0 {
		if !lo.Contains(featureFilter.FeatureIDs, f.ID) {
			return false
		}
	}

	// Filter by lookup key
	if featureFilter.LookupKey != "" {
		if f.LookupKey != featureFilter.LookupKey {
			return false
		}
	}

	// Filter by status - if no status is specified, only show published features
	if featureFilter.Status != nil {
		if f.Status != *featureFilter.Status {
			return false
		}
	} else if f.Status == types.StatusArchived {
		return false
	}

	// Filter by time range
	if featureFilter.TimeRangeFilter != nil {
		if featureFilter.StartTime != nil && f.CreatedAt.Before(*featureFilter.StartTime) {
			return false
		}
		if featureFilter.EndTime != nil && f.CreatedAt.After(*featureFilter.EndTime) {
			return false
		}
	}

	return true
}

// featureSortFn implements sorting logic for features
func featureSortFn(i, j *feature.Feature) bool {
	if i == nil || j == nil {
		return false
	}
	return i.CreatedAt.After(j.CreatedAt)
}

func (s *InMemoryFeatureStore) Create(ctx context.Context, f *feature.Feature) error {
	if f == nil {
		return fmt.Errorf("feature cannot be nil")
	}
	return s.InMemoryStore.Create(ctx, f.ID, f)
}

func (s *InMemoryFeatureStore) Get(ctx context.Context, id string) (*feature.Feature, error) {
	return s.InMemoryStore.Get(ctx, id)
}

func (s *InMemoryFeatureStore) List(ctx context.Context, filter *types.FeatureFilter) ([]*feature.Feature, error) {
	return s.InMemoryStore.List(ctx, filter, featureFilterFn, featureSortFn)
}

func (s *InMemoryFeatureStore) Count(ctx context.Context, filter *types.FeatureFilter) (int, error) {
	return s.InMemoryStore.Count(ctx, filter, featureFilterFn)
}

func (s *InMemoryFeatureStore) Update(ctx context.Context, f *feature.Feature) error {
	if f == nil {
		return fmt.Errorf("feature cannot be nil")
	}
	return s.InMemoryStore.Update(ctx, f.ID, f)
}

func (s *InMemoryFeatureStore) Delete(ctx context.Context, id string) error {
	return s.InMemoryStore.Delete(ctx, id)
}

// ListAll returns all features without pagination
func (s *InMemoryFeatureStore) ListAll(ctx context.Context, filter *types.FeatureFilter) ([]*feature.Feature, error) {
	// Create an unlimited filter
	unlimitedFilter := &types.FeatureFilter{
		QueryFilter:     types.NewNoLimitQueryFilter(),
		TimeRangeFilter: filter.TimeRangeFilter,
		FeatureIDs:      filter.FeatureIDs,
		LookupKey:       filter.LookupKey,
	}

	return s.List(ctx, unlimitedFilter)
}
