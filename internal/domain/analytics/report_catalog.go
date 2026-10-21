package analytics

import (
	"slices"

	"github.com/flexprice/flexprice/internal/types"
)

type ReportDimension struct {
	Key   string
	Label string
}

// ReportCatalogEntry describes one canned report.
type ReportCatalogEntry struct {
	Key             types.Metric
	Name            string
	Description     string
	Category        types.ReportCategory
	Kind            types.ReportKind
	ValueType       types.ReportValueType
	Improvement     types.ImprovementDirection
	Shapes          []types.Shape
	DefaultShape    types.Shape
	Dimensions      []ReportDimension
	Grains          []types.Grain
	SupportsCompare bool
}

var reportCatalog = []ReportCatalogEntry{
	{
		Key:          types.MetricInvoiceTotal,
		Name:         "Total billing",
		Description:  "Sum of finalized invoice totals in the period",
		Category:     types.ReportCategoryBilling,
		Kind:         types.ReportKindFlow,
		ValueType:    types.ReportValueTypeMoney,
		Improvement:  types.ImprovementHigher,
		Shapes:       []types.Shape{types.ShapeScalar, types.ShapeTimeseries, types.ShapeBreakdown},
		DefaultShape: types.ShapeTimeseries,
		Dimensions: []ReportDimension{
			{Key: "customer_id", Label: "Customer"},
			{Key: "invoice_type", Label: "Invoice type"},
		},
		Grains:          []types.Grain{types.GrainDay, types.GrainWeek, types.GrainMonth},
		SupportsCompare: true,
	},
}

func ReportCatalog() []ReportCatalogEntry {
	out := make([]ReportCatalogEntry, len(reportCatalog))
	for i, e := range reportCatalog {
		e.Shapes = slices.Clone(e.Shapes)
		e.Dimensions = slices.Clone(e.Dimensions)
		e.Grains = slices.Clone(e.Grains)
		out[i] = e
	}
	return out
}

func LookupReport(key string) (*ReportCatalogEntry, bool) {
	for _, e := range ReportCatalog() {
		if string(e.Key) == key {
			return &e, true
		}
	}
	return nil, false
}
