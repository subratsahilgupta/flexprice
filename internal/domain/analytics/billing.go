package analytics

import (
	"github.com/flexprice/flexprice/internal/types"
)

// BillingMetric describes one billing-family aggregate.
type BillingMetric struct {
	Metric  types.Metric
	Table   string
	TimeCol string
	Measure string
	// Aggregate name and argument.
	Agg   string
	Arg   string
	Where []string
	// Dimension name to source column.
	Dimensions map[string]string
	Shapes     []types.Shape
	// Forces currency into the group-by.
	Currency bool
	// Point-in-time metric; no time window.
	Snapshot bool
	// Replaces the generic builder.
	Build func(BuildInput) (*ReportQuery, error)
}

// BuildInput is handed to custom builders.
type BuildInput struct {
	RV     *ResolvedView
	DB     string
	Params map[string]string
	// Standard row invariants for alias.
	Scope func(alias string) string
	// Half-open window predicate on expr.
	TimeCond func(col string) string
	MaxRows  int
}

var billingMetrics = map[types.Metric]*BillingMetric{
	types.MetricInvoiceTotal: {
		Metric:  types.MetricInvoiceTotal,
		Table:   "invoices",
		TimeCol: "finalized_at",
		Measure: "sum(total)",
		Agg:     "sum",
		Arg:     "total",
		Where:   []string{"invoice_status = '" + string(types.InvoiceStatusFinalized) + "'"},
		Dimensions: map[string]string{
			"currency":     "currency",
			"customer_id":  "customer_id",
			"invoice_type": "invoice_type",
		},
		Shapes:   []types.Shape{types.ShapeScalar, types.ShapeTimeseries, types.ShapeBreakdown},
		Currency: true,
	},
}

func LookupBillingMetric(m types.Metric) (*BillingMetric, bool) {
	bm, ok := billingMetrics[m]
	return bm, ok
}
