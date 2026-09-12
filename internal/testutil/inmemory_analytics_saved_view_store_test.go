package testutil

import (
	"context"
	"testing"

	domainAnalytics "github.com/flexprice/flexprice/internal/domain/analytics"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCtx(tenantID string) context.Context {
	ctx := context.Background()
	return context.WithValue(ctx, types.CtxTenantID, tenantID)
}

func TestInMemoryAnalyticsSavedViewStore_CreateGet(t *testing.T) {
	store := NewInMemoryAnalyticsSavedViewStore()
	ctx := testCtx("tenant_1")

	view := &domainAnalytics.SavedView{
		ID:      "view_1",
		Name:    "My View",
		Version: 1,
		Definition: domainAnalytics.ViewDefinition{
			Name:    "My View",
			Shape:   domainAnalytics.ShapeBreakdown,
			Metrics: []string{"usage_quantity"},
		},
	}

	err := store.Create(ctx, view)
	require.NoError(t, err)
	assert.Equal(t, "tenant_1", view.TenantID)
	assert.Equal(t, types.StatusPublished, view.Status)

	got, err := store.Get(ctx, "view_1")
	require.NoError(t, err)
	assert.Equal(t, "My View", got.Name)
	assert.Equal(t, domainAnalytics.ShapeBreakdown, got.Definition.Shape)
}

func TestInMemoryAnalyticsSavedViewStore_CreateDuplicate(t *testing.T) {
	store := NewInMemoryAnalyticsSavedViewStore()
	ctx := testCtx("tenant_1")

	view := &domainAnalytics.SavedView{ID: "view_1", Name: "v1"}
	require.NoError(t, store.Create(ctx, view))

	err := store.Create(ctx, &domainAnalytics.SavedView{ID: "view_1", Name: "v2"})
	require.Error(t, err)
	assert.True(t, ierr.IsAlreadyExists(err))
}

func TestInMemoryAnalyticsSavedViewStore_GetNotFound(t *testing.T) {
	store := NewInMemoryAnalyticsSavedViewStore()
	ctx := testCtx("tenant_1")

	_, err := store.Get(ctx, "missing")
	require.Error(t, err)
	assert.True(t, ierr.IsNotFound(err))
}

func TestInMemoryAnalyticsSavedViewStore_ListFiltersByTenant(t *testing.T) {
	store := NewInMemoryAnalyticsSavedViewStore()

	require.NoError(t, store.Create(testCtx("tenant_1"), &domainAnalytics.SavedView{ID: "v1", Name: "a"}))
	require.NoError(t, store.Create(testCtx("tenant_2"), &domainAnalytics.SavedView{ID: "v2", Name: "b"}))

	views, err := store.List(testCtx("tenant_1"))
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "v1", views[0].ID)
}

func TestInMemoryAnalyticsSavedViewStore_Clear(t *testing.T) {
	store := NewInMemoryAnalyticsSavedViewStore()
	ctx := testCtx("tenant_1")

	require.NoError(t, store.Create(ctx, &domainAnalytics.SavedView{ID: "v1", Name: "a"}))
	store.Clear()

	views, err := store.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, views)
}
