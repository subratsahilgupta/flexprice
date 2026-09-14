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

// TestResolveVariables_RelativeTimeRange proves a literal relative-range
// token ("last_N_days") resolves to a [now-N*24h, now] UTC window.
func TestResolveVariables_RelativeTimeRange(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []string{"usage_quantity"},
		Time:    TimeSpecRaw{Range: "last_7_days", Grain: "day"},
	}
	before := time.Now().UTC()
	rv, err := ResolveVariables(def, map[string]any{})
	after := time.Now().UTC()
	require.NoError(t, err)

	assert.WithinDuration(t, before, rv.Time.To, after.Sub(before)+time.Second)
	assert.WithinDuration(t, rv.Time.To.Add(-7*24*time.Hour), rv.Time.From, time.Second)
	assert.Equal(t, "day", rv.Time.Grain)
}

// TestResolveVariables_RelativeTimeRangeHours proves the "last_N_hours" form.
func TestResolveVariables_RelativeTimeRangeHours(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []string{"usage_quantity"},
		Time:    TimeSpecRaw{Range: "last_24_hours", Grain: "hour"},
	}
	rv, err := ResolveVariables(def, map[string]any{})
	require.NoError(t, err)
	assert.WithinDuration(t, rv.Time.To.Add(-24*time.Hour), rv.Time.From, time.Second)
}

// TestResolveVariables_RelativeTimeRangeToday proves the "today" token.
func TestResolveVariables_RelativeTimeRangeToday(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []string{"usage_quantity"},
		Time:    TimeSpecRaw{Range: "today", Grain: "hour"},
	}
	rv, err := ResolveVariables(def, map[string]any{})
	require.NoError(t, err)
	now := time.Now().UTC()
	assert.Equal(t, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), rv.Time.From)
}

// TestResolveVariables_RelativeTimeRangeUnrecognizedToken proves an
// unrecognized relative token is a validation error, not a silent no-op.
func TestResolveVariables_RelativeTimeRangeUnrecognizedToken(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []string{"usage_quantity"},
		Time:    TimeSpecRaw{Range: "last_week", Grain: "day"},
	}
	_, err := ResolveVariables(def, map[string]any{})
	require.Error(t, err)
}

// TestResolveVariables_OmittedTimeDefaultsToLast7Days proves time is
// optional: a ViewDefinition with no Time set at all still resolves,
// defaulting to a 7-day window rather than erroring.
func TestResolveVariables_OmittedTimeDefaultsToLast7Days(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []string{"usage_quantity"},
		// Time intentionally left zero-valued.
	}
	rv, err := ResolveVariables(def, map[string]any{})
	require.NoError(t, err)
	assert.WithinDuration(t, rv.Time.To.Add(-7*24*time.Hour), rv.Time.From, time.Second)
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
