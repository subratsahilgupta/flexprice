package analytics

import (
	"slices"
	"testing"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportCatalog_AgreesWithRegistry(t *testing.T) {
	entries := ReportCatalog()
	require.NotEmpty(t, entries)
	for _, e := range entries {
		if e.Family == types.FamilyUsage {
			assert.NoError(t, types.MetricEventCount.Validate())
			assert.Contains(t, e.Shapes, e.DefaultShape)
			for _, d := range e.Dimensions {
				assert.NoError(t, ValidateDimensions([]string{d.Key}), "dimension %q of %q", d.Key, e.Key)
			}
			assert.False(t, e.SupportsCompare)
			continue
		}
		bm, ok := LookupBillingMetric(e.Key)
		require.True(t, ok, "catalog key %q has no billing metric", e.Key)
		if bm.Agg != "" {
			assert.Equal(t, bm.Agg+"("+bm.Arg+")", bm.Measure, "measure of %q", e.Key)
		} else {
			assert.False(t, e.SupportsCompare, "compare of %q", e.Key)
		}
		for _, s := range e.Shapes {
			assert.Contains(t, bm.Shapes, s, "shape %q of %q", s, e.Key)
		}
		assert.Contains(t, e.Shapes, e.DefaultShape)
		for _, d := range e.Dimensions {
			_, ok := bm.Dimensions[d.Key]
			assert.True(t, ok, "dimension %q of %q", d.Key, e.Key)
			assert.NotEmpty(t, d.Label)
		}
		for _, g := range e.Grains {
			assert.Contains(t, []types.Grain{types.GrainDay, types.GrainWeek, types.GrainMonth}, g)
		}
		assert.Contains(t, []string{"billing", "receivables", "customers", "credits", "usage"}, e.Category)
		assert.Contains(t, []string{"flow", "ratio", "snapshot"}, e.Kind)
		assert.Contains(t, []string{"money", "count", "days"}, e.ValueType)
		if e.Kind == "snapshot" {
			assert.False(t, e.SupportsCompare)
			assert.Empty(t, e.Grains)
		}
	}
}

func TestReportCatalog_BreakdownNeedsDimension(t *testing.T) {
	for _, e := range ReportCatalog() {
		if slices.Contains(e.Shapes, types.ShapeBreakdown) {
			assert.NotEmpty(t, e.Dimensions, "breakdown of %q", e.Key)
		}
	}
}

func TestReportCatalog_InvoiceTotal(t *testing.T) {
	e, ok := LookupReport("invoice_total")
	require.True(t, ok)
	assert.Equal(t, "Total billing", e.Name)
	assert.Equal(t, "flow", e.Kind)
	assert.Equal(t, "money", e.ValueType)
	assert.True(t, e.HigherIsBetter)
	assert.True(t, e.SupportsCompare)
	assert.Equal(t, types.ShapeTimeseries, e.DefaultShape)
	assert.True(t, slices.ContainsFunc(e.Dimensions, func(d ReportDimension) bool { return d.Key == "customer_id" }))
}

func TestReportCatalog_UnknownAndCopy(t *testing.T) {
	_, ok := LookupReport("nope")
	assert.False(t, ok)
	a := ReportCatalog()
	a[0].Name = "mutated"
	b := ReportCatalog()
	assert.NotEqual(t, "mutated", b[0].Name)
}
