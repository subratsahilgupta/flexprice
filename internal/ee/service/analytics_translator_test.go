package service

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/analytics"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateBreakdown_InjectsRLSAndGroupBy(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.CtxTenantID, "tenant_1")
	ctx = context.WithValue(ctx, types.CtxEnvironmentID, "env_1")

	rv := analytics.ResolvedView{
		Shape:      analytics.ShapeBreakdown,
		Metrics:    []string{"usage_quantity"},
		Dimensions: []string{"properties.region"},
		Filters:    []analytics.Filter{{Field: "meter_id", Op: "eq", Value: "meter_1"}},
		Time:       analytics.TimeSpec{From: time.Now().Add(-24 * time.Hour), To: time.Now(), Grain: "day"},
	}
	p, err := TranslateBreakdown(ctx, rv)
	require.NoError(t, err)
	assert.Equal(t, "tenant_1", p.TenantID)
	assert.Equal(t, "env_1", p.EnvironmentID)
	assert.Contains(t, p.MeterIDs, "meter_1")
	assert.Contains(t, p.GroupBy, "properties.region")
}

func TestTranslateBreakdown_RejectsIllegalDimension(t *testing.T) {
	ctx := context.Background()
	rv := analytics.ResolvedView{
		Shape: analytics.ShapeBreakdown, Metrics: []string{"usage_quantity"},
		Dimensions: []string{"properties.region; DROP TABLE"},
	}
	_, err := TranslateBreakdown(ctx, rv)
	require.Error(t, err)
}

func TestTranslateTimeseries_InjectsRLSAndFilters(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.CtxTenantID, "tenant_1")
	ctx = context.WithValue(ctx, types.CtxEnvironmentID, "env_1")

	rv := analytics.ResolvedView{
		Shape:   analytics.ShapeTimeseries,
		Metrics: []string{"usage_quantity"},
		Filters: []analytics.Filter{
			{Field: "meter_id", Op: "eq", Value: "meter_1"},
			{Field: "customer_id", Op: "eq", Value: "cust_1"},
			{Field: "source", Op: "eq", Value: "api"},
		},
		Time: analytics.TimeSpec{From: time.Now().Add(-24 * time.Hour), To: time.Now(), Grain: "hour"},
	}
	p, err := TranslateTimeseries(ctx, rv)
	require.NoError(t, err)
	assert.Equal(t, "tenant_1", p.TenantID)
	assert.Equal(t, "env_1", p.EnvironmentID)
	assert.Contains(t, p.MeterIDs, "meter_1")
	assert.Contains(t, p.ExternalCustomerIDs, "cust_1")
	assert.Contains(t, p.Sources, "api")
	assert.Equal(t, types.WindowSizeHour, p.WindowSize)
	assert.True(t, p.UseFinal)
}

func TestTranslateTimeseries_RejectsIllegalDimension(t *testing.T) {
	ctx := context.Background()
	rv := analytics.ResolvedView{
		Shape:      analytics.ShapeTimeseries,
		Metrics:    []string{"usage_quantity"},
		Dimensions: []string{"properties.region; DROP TABLE"},
	}
	_, err := TranslateTimeseries(ctx, rv)
	require.Error(t, err)
}

func TestTranslateBreakdown_PropertyFilterFallsThrough(t *testing.T) {
	ctx := context.Background()
	rv := analytics.ResolvedView{
		Shape:   analytics.ShapeBreakdown,
		Metrics: []string{"usage_quantity"},
		Filters: []analytics.Filter{{Field: "model", Op: "eq", Value: "gpt-4"}},
	}
	p, err := TranslateBreakdown(ctx, rv)
	require.NoError(t, err)
	assert.Contains(t, p.PropertyFilters["model"], "gpt-4")
}
