package analytics

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveVariables_DropsUnsuppliedOptionalFilter(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []Metric{MetricUsageQuantity},
		Filters: []*Filter{
			{Field: "meter_id", Op: types.EQUAL, Value: []string{"{{meter}}"}},
			{Field: "customer_id", Op: types.IN, Value: []string{"{{customers}}"}, Optional: true},
		},
		Time:      TimeSpecRaw{Range: "{{date_range}}", Grain: GrainDay},
		Variables: []*Variable{{Name: "meter", Type: VariableTypeString, Required: true}, {Name: "customers", Type: VariableTypeStringList}},
	}
	rv, err := ResolveVariables(&def, map[string][]string{
		"meter":      {"meter_1"},
		"date_range": {"2026-08-01..2026-09-01"},
	})
	require.NoError(t, err)
	assert.Len(t, rv.Filters, 1) // optional customers filter dropped
	assert.Equal(t, []string{"meter_1"}, rv.Filters[0].Value)
	assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), rv.Time.From)
}

func TestResolveVariables_MissingRequiredVar(t *testing.T) {
	def := ViewDefinition{
		Shape: ShapeTimeseries, Metrics: []Metric{MetricUsageQuantity},
		Variables: []*Variable{{Name: "meter", Type: VariableTypeString, Required: true}},
	}
	_, err := ResolveVariables(&def, map[string][]string{})
	require.Error(t, err)
}

func TestValidate_RejectsUnknownShape(t *testing.T) {
	def := ViewDefinition{Shape: Shape("pie"), Metrics: []Metric{MetricUsageQuantity}}
	require.Error(t, def.Validate())
}

func TestResolveVariables_MissingRequiredFilterValue(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []Metric{MetricUsageQuantity},
		Filters: []*Filter{
			{Field: "meter_id", Op: types.EQUAL, Value: []string{"{{meter}}"}},
		},
		Time: TimeSpecRaw{Range: "2026-08-01..2026-09-01", Grain: GrainDay},
	}
	_, err := ResolveVariables(&def, map[string][]string{})
	require.Error(t, err)
}

func TestResolveVariables_LiteralFilterValuePassesThrough(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []Metric{MetricUsageQuantity},
		Filters: []*Filter{
			{Field: "status", Op: types.EQUAL, Value: []string{"active"}},
		},
		Time: TimeSpecRaw{Range: "2026-08-01..2026-09-01", Grain: GrainDay},
	}
	rv, err := ResolveVariables(&def, map[string][]string{})
	require.NoError(t, err)
	require.Len(t, rv.Filters, 1)
	assert.Equal(t, []string{"active"}, rv.Filters[0].Value)
}

func TestResolveVariables_TimeRangeNotSupplied(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []Metric{MetricUsageQuantity},
		Time:    TimeSpecRaw{Range: "{{date_range}}", Grain: GrainDay},
	}
	_, err := ResolveVariables(&def, map[string][]string{})
	require.Error(t, err)
}

func TestResolveVariables_TimeRangeEmptyVariable(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []Metric{MetricUsageQuantity},
		Time:    TimeSpecRaw{Range: "{{date_range}}", Grain: GrainDay},
	}
	_, err := ResolveVariables(&def, map[string][]string{"date_range": {}})
	require.Error(t, err)
}

func TestResolveVariables_InvalidDateFormat(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []Metric{MetricUsageQuantity},
		Time:    TimeSpecRaw{Range: "{{date_range}}", Grain: GrainDay},
	}
	_, err := ResolveVariables(&def, map[string][]string{
		"date_range": {"08/01/2026..2026-09-01"},
	})
	require.Error(t, err)
}

func TestResolveVariables_LiteralTimeRange(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []Metric{MetricUsageQuantity},
		Time:    TimeSpecRaw{Range: "2026-08-01..2026-09-01", Grain: GrainMonth},
	}
	rv, err := ResolveVariables(&def, map[string][]string{})
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), rv.Time.From)
	assert.Equal(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), rv.Time.To)
	assert.Equal(t, GrainMonth, rv.Time.Grain)
}

func TestResolveVariables_InvalidDefinitionSurfacesValidateError(t *testing.T) {
	def := ViewDefinition{Shape: Shape("pie"), Metrics: []Metric{MetricUsageQuantity}}
	_, err := ResolveVariables(&def, map[string][]string{})
	require.Error(t, err)
}

// TestResolveVariables_RelativeTimeRange proves a literal relative-range
// token ("last_N_days") resolves to a [now-N*24h, now] UTC window.
func TestResolveVariables_RelativeTimeRange(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []Metric{MetricUsageQuantity},
		Time:    TimeSpecRaw{Range: "last_7_days", Grain: GrainDay},
	}
	before := time.Now().UTC()
	rv, err := ResolveVariables(&def, map[string][]string{})
	after := time.Now().UTC()
	require.NoError(t, err)

	assert.WithinDuration(t, before, rv.Time.To, after.Sub(before)+time.Second)
	assert.WithinDuration(t, rv.Time.To.Add(-7*24*time.Hour), rv.Time.From, time.Second)
	assert.Equal(t, GrainDay, rv.Time.Grain)
}

// TestResolveVariables_RelativeTimeRangeHours proves the "last_N_hours" form.
func TestResolveVariables_RelativeTimeRangeHours(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []Metric{MetricUsageQuantity},
		Time:    TimeSpecRaw{Range: "last_24_hours", Grain: GrainHour},
	}
	rv, err := ResolveVariables(&def, map[string][]string{})
	require.NoError(t, err)
	assert.WithinDuration(t, rv.Time.To.Add(-24*time.Hour), rv.Time.From, time.Second)
}

// TestResolveVariables_RelativeTimeRangeToday proves the "today" token.
func TestResolveVariables_RelativeTimeRangeToday(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []Metric{MetricUsageQuantity},
		Time:    TimeSpecRaw{Range: "today", Grain: GrainHour},
	}
	rv, err := ResolveVariables(&def, map[string][]string{})
	require.NoError(t, err)
	now := time.Now().UTC()
	assert.Equal(t, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), rv.Time.From)
}

// TestResolveVariables_RelativeTimeRangeUnrecognizedToken proves an
// unrecognized relative token is a validation error, not a silent no-op.
func TestResolveVariables_RelativeTimeRangeUnrecognizedToken(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []Metric{MetricUsageQuantity},
		Time:    TimeSpecRaw{Range: "last_week", Grain: GrainDay},
	}
	_, err := ResolveVariables(&def, map[string][]string{})
	require.Error(t, err)
}

// TestResolveVariables_OmittedTimeDefaultsToLast7Days proves time is
// optional: a ViewDefinition with no Time set at all still resolves,
// defaulting to a 7-day window rather than erroring.
func TestResolveVariables_OmittedTimeDefaultsToLast7Days(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []Metric{MetricUsageQuantity},
		// Time intentionally left zero-valued.
	}
	rv, err := ResolveVariables(&def, map[string][]string{})
	require.NoError(t, err)
	assert.WithinDuration(t, rv.Time.To.Add(-7*24*time.Hour), rv.Time.From, time.Second)
}

func TestResolveVariables_PreservesShapeMetricsDimensionsSortLimit(t *testing.T) {
	def := ViewDefinition{
		Shape:      ShapeBreakdown,
		Metrics:    []Metric{MetricUsageQuantity, MetricEventCount},
		Dimensions: []string{"customer_id"},
		Sort:       []*SortSpec{{Field: "usage_quantity", Dir: types.SortDirectionDesc}},
		Limit:      25,
		Time:       TimeSpecRaw{Range: "2026-08-01..2026-09-01", Grain: GrainDay},
	}
	rv, err := ResolveVariables(&def, map[string][]string{})
	require.NoError(t, err)
	assert.Equal(t, ShapeBreakdown, rv.Shape)
	assert.Equal(t, []Metric{MetricUsageQuantity, MetricEventCount}, rv.Metrics)
	assert.Equal(t, []string{"customer_id"}, rv.Dimensions)
	assert.Equal(t, []*SortSpec{{Field: "usage_quantity", Dir: types.SortDirectionDesc}}, rv.Sort)
	assert.Equal(t, 25, rv.Limit)
}
