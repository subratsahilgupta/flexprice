package analytics

import (
	"github.com/flexprice/flexprice/internal/types"
)

// BillingMetric describes one billing-family aggregate.
type BillingMetric struct {
	Metric     types.Metric
	Dimensions []string
	Shapes     []types.Shape
	// Forces currency into the group-by.
	Currency bool
}

var billingMetrics = map[types.Metric]*BillingMetric{
	types.MetricInvoiceTotal: {
		Metric:     types.MetricInvoiceTotal,
		Dimensions: []string{"currency", "customer_id", "invoice_type"},
		Shapes:     []types.Shape{types.ShapeScalar, types.ShapeTimeseries, types.ShapeBreakdown},
		Currency:   true,
	},
}

func LookupBillingMetric(m types.Metric) (*BillingMetric, bool) {
	bm, ok := billingMetrics[m]
	return bm, ok
}

// BillingMetrics lists every registered metric key.
func BillingMetrics() []types.Metric {
	out := make([]types.Metric, 0, len(billingMetrics))
	for m := range billingMetrics {
		out = append(out, m)
	}
	return out
}
