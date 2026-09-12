package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestShapeBreakdown_ColumnsAndRows(t *testing.T) {
	items := []events.MeterUsageDetailedResult{
		{Properties: map[string]string{"region": "us"}, TotalUsage: decimal.NewFromInt(60000)},
		{Properties: map[string]string{"region": "eu"}, TotalUsage: decimal.NewFromInt(30000)},
	}
	got := ShapeBreakdown(items, []string{"properties.region"})
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
	got := ShapeBreakdown(nil, []string{"properties.region"})
	assert.Len(t, got.Columns, 2)
	assert.Len(t, got.Rows, 0)
}

func TestShapeBreakdown_NoDims(t *testing.T) {
	items := []events.MeterUsageDetailedResult{
		{TotalUsage: decimal.NewFromInt(100)},
	}
	got := ShapeBreakdown(items, nil)
	assert.Len(t, got.Columns, 1)
	assert.Equal(t, "usage_quantity", got.Columns[0].Name)
	assert.Equal(t, []any{"100"}, got.Rows[0])
}

func TestShapeTimeseries_ColumnsAndRows(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	res := &events.MeterUsageAggregationResult{
		TotalValue: decimal.NewFromInt(150),
		Points: []events.MeterUsageResult{
			{WindowStart: t1, Value: decimal.NewFromInt(100)},
			{WindowStart: t2, Value: decimal.NewFromInt(50)},
		},
	}
	got := ShapeTimeseries(res)
	assert.Equal(t, "window_start", got.Columns[0].Name)
	assert.Equal(t, "dimension", got.Columns[0].Role)
	assert.Equal(t, "usage_quantity", got.Columns[1].Name)
	assert.Equal(t, "metric", got.Columns[1].Role)
	assert.Len(t, got.Rows, 2)
	assert.Equal(t, t1, got.Rows[0][0])
	assert.Equal(t, "100", got.Rows[0][1])
	assert.Equal(t, "150", got.Meta["total"])
	assert.Equal(t, "meter_usage", got.Meta["query_source"])
}

func TestShapeTimeseries_EmptyPoints(t *testing.T) {
	res := &events.MeterUsageAggregationResult{TotalValue: decimal.Zero}
	got := ShapeTimeseries(res)
	assert.Len(t, got.Rows, 0)
	assert.Equal(t, "0", got.Meta["total"])
}
