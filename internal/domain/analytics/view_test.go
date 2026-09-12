package analytics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShapeConstants(t *testing.T) {
	assert.Equal(t, Shape("timeseries"), ShapeTimeseries)
	assert.Equal(t, Shape("breakdown"), ShapeBreakdown)
}

func TestValidate_AcceptsTimeseriesAndBreakdown(t *testing.T) {
	for _, shape := range []Shape{ShapeTimeseries, ShapeBreakdown} {
		def := ViewDefinition{Shape: shape, Metrics: []string{"usage_quantity"}}
		require.NoError(t, def.Validate())
	}
}

func TestValidate_RejectsEmptyMetrics(t *testing.T) {
	def := ViewDefinition{Shape: ShapeTimeseries, Metrics: []string{}}
	require.Error(t, def.Validate())
}

func TestValidate_RejectsNilMetrics(t *testing.T) {
	def := ViewDefinition{Shape: ShapeBreakdown}
	require.Error(t, def.Validate())
}

func TestNewViewDefinition_SetsAllFields(t *testing.T) {
	filters := []Filter{{Field: "meter_id", Op: "eq", Value: "meter_1"}}
	sort := []SortSpec{{Field: "usage_quantity", Dir: "desc"}}
	variables := []Variable{{Name: "meter", Type: "string", Required: true}}
	timeSpec := TimeSpecRaw{Range: "{{date_range}}", Grain: "day"}

	def := NewViewDefinition(
		"my_view",
		ShapeBreakdown,
		[]string{"usage_quantity"},
		[]string{"customer_id"},
		filters,
		timeSpec,
		sort,
		10,
		variables,
	)

	assert.Equal(t, "my_view", def.Name)
	assert.Equal(t, ShapeBreakdown, def.Shape)
	assert.Equal(t, []string{"usage_quantity"}, def.Metrics)
	assert.Equal(t, []string{"customer_id"}, def.Dimensions)
	assert.Equal(t, filters, def.Filters)
	assert.Equal(t, timeSpec, def.Time)
	assert.Equal(t, sort, def.Sort)
	assert.Equal(t, 10, def.Limit)
	assert.Equal(t, variables, def.Variables)
	require.NoError(t, def.Validate())
}
