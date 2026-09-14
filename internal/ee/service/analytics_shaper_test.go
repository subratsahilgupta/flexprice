package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestShapeBreakdown_ColumnsAndRows(t *testing.T) {
	items := []dto.UsageAnalyticItem{
		{Properties: map[string]string{"region": "us"}, TotalUsage: decimal.NewFromInt(60000)},
		{Properties: map[string]string{"region": "eu"}, TotalUsage: decimal.NewFromInt(30000)},
	}
	got := shapeBreakdown(items, []string{"properties.region"})
	assert.Equal(t, "properties.region", got.Columns[0].Name)
	assert.Equal(t, "dimension", got.Columns[0].Role)
	assert.Equal(t, "usage_quantity", got.Columns[1].Name)
	assert.Equal(t, "metric", got.Columns[1].Role)
	assert.Len(t, got.Rows, 2)
	assert.Equal(t, "us", got.Rows[0][0])
	assert.Equal(t, "60000", got.Rows[0][1])
	assert.Equal(t, "eu", got.Rows[1][0])
	assert.Equal(t, "meter_usage", got.Meta["query_source"])
}

func TestShapeBreakdown_EmptyItems(t *testing.T) {
	got := shapeBreakdown(nil, []string{"properties.region"})
	assert.Len(t, got.Columns, 2)
	assert.Len(t, got.Rows, 0)
}

func TestShapeBreakdown_NoDims(t *testing.T) {
	items := []dto.UsageAnalyticItem{
		{TotalUsage: decimal.NewFromInt(100)},
	}
	got := shapeBreakdown(items, nil)
	assert.Len(t, got.Columns, 1)
	assert.Equal(t, "usage_quantity", got.Columns[0].Name)
	assert.Equal(t, []any{"100"}, got.Rows[0])
}

// TestShapeBreakdown_StructuralDimsPopulate proves meter_id/source/
// external_customer_id (not just properties.*) populate correctly — the old
// shaper only read item.Properties.
func TestShapeBreakdown_StructuralDimsPopulate(t *testing.T) {
	items := []dto.UsageAnalyticItem{
		{
			MeterID:            "meter_1",
			Source:             "api",
			ExternalCustomerID: "cust_1",
			TotalUsage:         decimal.NewFromInt(42),
		},
	}
	got := shapeBreakdown(items, []string{"meter_id", "source", "customer_id"})
	assert.Equal(t, []any{"meter_1", "api", "cust_1", "42"}, got.Rows[0])
}

func TestShapeTimeseries_ColumnsAndRows(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	items := []dto.UsageAnalyticItem{
		{
			MeterID: "meter_1",
			Points: []dto.UsageAnalyticPoint{
				{Timestamp: t1, Usage: decimal.NewFromInt(100)},
				{Timestamp: t2, Usage: decimal.NewFromInt(50)},
			},
		},
	}
	got := shapeTimeseries(items, nil)
	assert.Equal(t, "window_start", got.Columns[0].Name)
	assert.Equal(t, "dimension", got.Columns[0].Role)
	assert.Equal(t, "usage_quantity", got.Columns[1].Name)
	assert.Equal(t, "metric", got.Columns[1].Role)
	assert.Len(t, got.Rows, 2)
	assert.Equal(t, t1, got.Rows[0][0])
	assert.Equal(t, "100", got.Rows[0][1])
	assert.Equal(t, "meter_usage", got.Meta["query_source"])
	_, hasTotal := got.Meta["total"]
	assert.False(t, hasTotal, "timeseries meta must not include a cross-bucket total")
}

func TestShapeTimeseries_EmptyPoints(t *testing.T) {
	got := shapeTimeseries(nil, nil)
	assert.Len(t, got.Rows, 0)
	_, hasTotal := got.Meta["total"]
	assert.False(t, hasTotal, "timeseries meta must not include a cross-bucket total")
}

// TestShapeTimeseries_WithDimensionAndWindow proves a split-by dimension is
// carried on every bucketed row alongside window_start, one row per
// (item, point) pair — buckets from different group combos stay distinct.
func TestShapeTimeseries_WithDimensionAndWindow(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []dto.UsageAnalyticItem{
		{
			Properties: map[string]string{"region": "us"},
			Points:     []dto.UsageAnalyticPoint{{Timestamp: t1, Usage: decimal.NewFromInt(10)}},
		},
		{
			Properties: map[string]string{"region": "eu"},
			Points:     []dto.UsageAnalyticPoint{{Timestamp: t1, Usage: decimal.NewFromInt(20)}},
		},
	}
	got := shapeTimeseries(items, []string{"properties.region"})
	assert.Equal(t, []string{"window_start", "properties.region", "usage_quantity"}, columnNames(got.Columns))
	assert.Len(t, got.Rows, 2)
	assert.Equal(t, []any{t1, "us", "10"}, got.Rows[0])
	assert.Equal(t, []any{t1, "eu", "20"}, got.Rows[1])
	_, hasTotal := got.Meta["total"]
	assert.False(t, hasTotal, "timeseries meta must not include a cross-bucket total")
}

func columnNames(cols []*dto.AnalyticsColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Name
	}
	return out
}
