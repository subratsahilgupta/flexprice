package analytics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveVariables_DropsUnsuppliedOptionalFilter(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []string{"usage_quantity"},
		Filters: []Filter{
			{Field: "meter_id", Op: "eq", Value: "{{meter}}"},
			{Field: "customer_id", Op: "in", Value: "{{customers}}", Optional: true},
		},
		Time:      TimeSpecRaw{Range: "{{date_range}}", Grain: "day"},
		Variables: []Variable{{Name: "meter", Type: "string", Required: true}, {Name: "customers", Type: "string_list"}},
	}
	rv, err := ResolveVariables(def, map[string]any{
		"meter":      "meter_1",
		"date_range": map[string]any{"from": "2026-08-01", "to": "2026-09-01"},
	})
	require.NoError(t, err)
	assert.Len(t, rv.Filters, 1) // optional customers filter dropped
	assert.Equal(t, "meter_1", rv.Filters[0].Value)
	assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), rv.Time.From)
}

func TestResolveVariables_MissingRequiredVar(t *testing.T) {
	def := ViewDefinition{
		Shape: ShapeTimeseries, Metrics: []string{"usage_quantity"},
		Variables: []Variable{{Name: "meter", Type: "string", Required: true}},
	}
	_, err := ResolveVariables(def, map[string]any{})
	require.Error(t, err)
}

func TestValidate_RejectsUnknownShape(t *testing.T) {
	def := ViewDefinition{Shape: Shape("pie"), Metrics: []string{"usage_quantity"}}
	require.Error(t, def.Validate())
}

func TestResolveVariables_MissingRequiredFilterValue(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []string{"usage_quantity"},
		Filters: []Filter{
			{Field: "meter_id", Op: "eq", Value: "{{meter}}"},
		},
		Time: TimeSpecRaw{Range: map[string]any{"from": "2026-08-01", "to": "2026-09-01"}, Grain: "day"},
	}
	_, err := ResolveVariables(def, map[string]any{})
	require.Error(t, err)
}

func TestResolveVariables_LiteralFilterValuePassesThrough(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []string{"usage_quantity"},
		Filters: []Filter{
			{Field: "status", Op: "eq", Value: "active"},
		},
		Time: TimeSpecRaw{Range: map[string]any{"from": "2026-08-01", "to": "2026-09-01"}, Grain: "day"},
	}
	rv, err := ResolveVariables(def, map[string]any{})
	require.NoError(t, err)
	require.Len(t, rv.Filters, 1)
	assert.Equal(t, "active", rv.Filters[0].Value)
}

func TestResolveVariables_TimeRangeNotSupplied(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []string{"usage_quantity"},
		Time:    TimeSpecRaw{Range: "{{date_range}}", Grain: "day"},
	}
	_, err := ResolveVariables(def, map[string]any{})
	require.Error(t, err)
}

func TestResolveVariables_TimeRangeWrongType(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []string{"usage_quantity"},
		Time:    TimeSpecRaw{Range: "{{date_range}}", Grain: "day"},
	}
	_, err := ResolveVariables(def, map[string]any{"date_range": "not-a-map"})
	require.Error(t, err)
}

func TestResolveVariables_InvalidDateFormat(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []string{"usage_quantity"},
		Time:    TimeSpecRaw{Range: "{{date_range}}", Grain: "day"},
	}
	_, err := ResolveVariables(def, map[string]any{
		"date_range": map[string]any{"from": "08/01/2026", "to": "2026-09-01"},
	})
	require.Error(t, err)
}

func TestResolveVariables_LiteralTimeRange(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []string{"usage_quantity"},
		Time:    TimeSpecRaw{Range: map[string]any{"from": "2026-08-01", "to": "2026-09-01"}, Grain: "month"},
	}
	rv, err := ResolveVariables(def, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), rv.Time.From)
	assert.Equal(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), rv.Time.To)
	assert.Equal(t, "month", rv.Time.Grain)
}

func TestResolveVariables_InvalidDefinitionSurfacesValidateError(t *testing.T) {
	def := ViewDefinition{Shape: Shape("pie"), Metrics: []string{"usage_quantity"}}
	_, err := ResolveVariables(def, map[string]any{})
	require.Error(t, err)
}

func TestResolveVariables_PreservesShapeMetricsDimensionsSortLimit(t *testing.T) {
	def := ViewDefinition{
		Shape:      ShapeBreakdown,
		Metrics:    []string{"usage_quantity", "usage_cost"},
		Dimensions: []string{"customer_id"},
		Sort:       []SortSpec{{Field: "usage_quantity", Dir: "desc"}},
		Limit:      25,
		Time:       TimeSpecRaw{Range: map[string]any{"from": "2026-08-01", "to": "2026-09-01"}, Grain: "day"},
	}
	rv, err := ResolveVariables(def, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, ShapeBreakdown, rv.Shape)
	assert.Equal(t, []string{"usage_quantity", "usage_cost"}, rv.Metrics)
	assert.Equal(t, []string{"customer_id"}, rv.Dimensions)
	assert.Equal(t, []SortSpec{{Field: "usage_quantity", Dir: "desc"}}, rv.Sort)
	assert.Equal(t, 25, rv.Limit)
}
