package analytics

import (
	"testing"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShapeConstants(t *testing.T) {
	assert.Equal(t, Shape("timeseries"), ShapeTimeseries)
	assert.Equal(t, Shape("breakdown"), ShapeBreakdown)
}

func TestValidate_AcceptsTimeseriesAndBreakdown(t *testing.T) {
	for _, shape := range []Shape{ShapeTimeseries, ShapeBreakdown} {
		def := ViewDefinition{Shape: shape, Metrics: []Metric{MetricUsageQuantity}}
		require.NoError(t, def.Validate())
	}
}

func TestValidate_RejectsEmptyMetrics(t *testing.T) {
	def := ViewDefinition{Shape: ShapeTimeseries, Metrics: []Metric{}}
	require.Error(t, def.Validate())
}

func TestValidate_RejectsNilMetrics(t *testing.T) {
	def := ViewDefinition{Shape: ShapeBreakdown}
	require.Error(t, def.Validate())
}

func TestValidate_RejectsInvalidMetric(t *testing.T) {
	def := ViewDefinition{Shape: ShapeBreakdown, Metrics: []Metric{Metric("total_cost")}}
	require.Error(t, def.Validate())
}

func TestValidate_RejectsInvalidDimension(t *testing.T) {
	def := ViewDefinition{Shape: ShapeBreakdown, Metrics: []Metric{MetricUsageQuantity}, Dimensions: []string{"plan_id"}}
	require.Error(t, def.Validate())
}

func TestValidate_AcceptsAllowlistedDimensions(t *testing.T) {
	def := ViewDefinition{
		Shape:      ShapeBreakdown,
		Metrics:    []Metric{MetricUsageQuantity},
		Dimensions: []string{"meter_id", "source", "external_customer_id", "customer_id", "properties.region"},
	}
	require.NoError(t, def.Validate())
}

func TestValidate_RejectsInvalidGrain(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeTimeseries,
		Metrics: []Metric{MetricUsageQuantity},
		Time:    TimeSpecRaw{Grain: Grain("fortnight")},
	}
	require.Error(t, def.Validate())
}

func TestValidate_RejectsInvalidSortDirection(t *testing.T) {
	def := ViewDefinition{
		Shape:   ShapeBreakdown,
		Metrics: []Metric{MetricUsageQuantity},
		Sort:    []*SortSpec{{Field: "usage_quantity", Dir: types.SortDirection("sideways")}},
	}
	require.Error(t, def.Validate())
}

func TestValidate_RejectsInvalidVariableType(t *testing.T) {
	def := ViewDefinition{
		Shape:     ShapeBreakdown,
		Metrics:   []Metric{MetricUsageQuantity},
		Variables: []*Variable{{Name: "x", Type: VariableType("weird")}},
	}
	require.Error(t, def.Validate())
}

func TestNewViewDefinition_SetsAllFields(t *testing.T) {
	filters := []*Filter{{Field: "meter_id", Op: types.EQUAL, Value: []string{"meter_1"}}}
	sort := []*SortSpec{{Field: "usage_quantity", Dir: types.SortDirectionDesc}}
	variables := []*Variable{{Name: "meter", Type: VariableTypeString, Required: true}}
	timeSpec := TimeSpecRaw{Range: "{{date_range}}", Grain: GrainDay}

	def := NewViewDefinition(
		"my_view",
		ShapeBreakdown,
		[]Metric{MetricUsageQuantity},
		[]string{"customer_id"},
		filters,
		timeSpec,
		sort,
		10,
		variables,
	)

	assert.Equal(t, "my_view", def.Name)
	assert.Equal(t, ShapeBreakdown, def.Shape)
	assert.Equal(t, []Metric{MetricUsageQuantity}, def.Metrics)
	assert.Equal(t, []string{"customer_id"}, def.Dimensions)
	assert.Equal(t, filters, def.Filters)
	assert.Equal(t, timeSpec, def.Time)
	assert.Equal(t, sort, def.Sort)
	assert.Equal(t, 10, def.Limit)
	assert.Equal(t, variables, def.Variables)
	require.NoError(t, def.Validate())
}

func TestGrain_ToWindowSize(t *testing.T) {
	assert.Equal(t, types.WindowSizeHour, GrainHour.ToWindowSize())
	assert.Equal(t, types.WindowSizeDay, GrainDay.ToWindowSize())
	assert.Equal(t, types.WindowSizeWeek, GrainWeek.ToWindowSize())
	assert.Equal(t, types.WindowSizeMonth, GrainMonth.ToWindowSize())
	assert.Equal(t, types.WindowSize(""), Grain("").ToWindowSize())
}

func TestValidateDimensions_RejectsEmptyPropertyField(t *testing.T) {
	require.Error(t, ValidateDimensions([]string{"properties."}))
}
