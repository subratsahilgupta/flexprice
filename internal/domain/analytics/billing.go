package analytics

import (
	"strings"

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
	// Half-open window predicate on col.
	TimeCond func(col string) string
	MaxRows  int
}

const agingBucketExpr = "multiIf(nullIf(due_date, toDateTime64(0, 6)) IS NULL, 'No due date', " +
	"dateDiff('day', due_date, now64(6, 'UTC'), 'UTC') <= 0, 'Current', " +
	"dateDiff('day', due_date, now64(6, 'UTC'), 'UTC') <= 30, '1-30', " +
	"dateDiff('day', due_date, now64(6, 'UTC'), 'UTC') <= 60, '31-60', " +
	"dateDiff('day', due_date, now64(6, 'UTC'), 'UTC') <= 90, '61-90', '90+')"

// invoiceBillingTime: billed period; one-offs use issue date.
func invoiceBillingTime(prefix string) string {
	return "coalesce(nullIf(" + prefix + "period_start, " + chEpoch + "), " + prefix + "issue_date)"
}

func invoiceIssueTime(prefix string) string {
	return prefix + "issue_date"
}

func invoiceTotalMetric(m types.Metric, timeCol string) *BillingMetric {
	return &BillingMetric{
		Metric:  m,
		Table:   "invoices",
		TimeCol: timeCol,
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
	}
}

func netBillingMetric(m types.Metric, invoiceTime func(prefix string) string) *BillingMetric {
	return &BillingMetric{
		Metric:     m,
		Dimensions: map[string]string{"currency": "currency"},
		Shapes:     []types.Shape{types.ShapeScalar, types.ShapeTimeseries},
		Currency:   true,
		Build: func(in BuildInput) (*ReportQuery, error) {
			return buildNetBilling(m, invoiceTime, in)
		},
	}
}

var (
	paidIn = "payment_status IN ('" + string(types.PaymentStatusSucceeded) + "','" + string(types.PaymentStatusOverpaid) + "')"
	// Open invoices with a balance.
	receivableWhere = []string{
		"invoice_status = '" + string(types.InvoiceStatusFinalized) + "'",
		"amount_remaining > 0",
	}
	creditTopupReasons = []types.TransactionReason{
		types.TransactionReasonPurchasedCreditDirect,
		types.TransactionReasonPurchasedCreditInvoiced,
		types.TransactionReasonPurchasedCreditBonus,
		types.TransactionReasonFreeCredit,
		types.TransactionReasonSubscriptionCredit,
	}
)

func quotedList[T ~string](vs []T) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = "'" + string(v) + "'"
	}
	return strings.Join(q, ",")
}

var billingMetrics = map[types.Metric]*BillingMetric{
	types.MetricTotalReceivables: {
		Metric:     types.MetricTotalReceivables,
		Table:      "invoices",
		TimeCol:    "finalized_at",
		Measure:    "sum(amount_remaining)",
		Agg:        "sum",
		Arg:        "amount_remaining",
		Where:      receivableWhere,
		Dimensions: map[string]string{"currency": "currency"},
		Shapes:     []types.Shape{types.ShapeScalar, types.ShapeBreakdown},
		Currency:   true,
		Snapshot:   true,
	},
	types.MetricReceivablesAging: {
		Metric:  types.MetricReceivablesAging,
		Table:   "invoices",
		TimeCol: "finalized_at",
		Measure: "sum(amount_remaining)",
		Agg:     "sum",
		Arg:     "amount_remaining",
		Where:   receivableWhere,
		Dimensions: map[string]string{
			"aging_bucket": agingBucketExpr,
			"customer_id":  "customer_id",
			"currency":     "currency",
		},
		Shapes:   []types.Shape{types.ShapeBreakdown},
		Currency: true,
		Snapshot: true,
	},
	types.MetricDaysToPay: {
		Metric:  types.MetricDaysToPay,
		Table:   "invoices",
		TimeCol: "paid_at",
		Measure: "avg(dateDiff('day', issue_date, paid_at, 'UTC'))",
		Agg:     "avg",
		Arg:     "dateDiff('day', issue_date, paid_at, 'UTC')",
		Where: []string{
			paidIn,
			"paid_at > " + chEpoch,
			"issue_date > " + chEpoch,
		},
		Dimensions: map[string]string{"customer_id": "customer_id", "currency": "currency"},
		Shapes:     []types.Shape{types.ShapeScalar, types.ShapeTimeseries, types.ShapeBreakdown},
		Currency:   true,
	},
	types.MetricARPU: {
		Metric:     types.MetricARPU,
		Table:      "invoices",
		TimeCol:    "paid_at",
		Measure:    "sum(amount_paid) / uniqExact(customer_id)",
		Where:      []string{paidIn, "paid_at > " + chEpoch},
		Dimensions: map[string]string{"currency": "currency"},
		Shapes:     []types.Shape{types.ShapeScalar, types.ShapeTimeseries},
		Currency:   true,
	},
	types.MetricARPUBilled: {
		Metric:     types.MetricARPUBilled,
		Table:      "invoices",
		TimeCol:    invoiceBillingTime(""),
		Measure:    "sum(total) / uniqExact(customer_id)",
		Where:      []string{"invoice_status = '" + string(types.InvoiceStatusFinalized) + "'"},
		Dimensions: map[string]string{"currency": "currency"},
		Shapes:     []types.Shape{types.ShapeScalar, types.ShapeTimeseries},
		Currency:   true,
	},
	types.MetricCreditConsumption: {
		Metric:  types.MetricCreditConsumption,
		Table:   "wallet_transactions",
		TimeCol: "created_at",
		Measure: "sum(amount)",
		Agg:     "sum",
		Arg:     "amount",
		Where: []string{
			"type = '" + string(types.TransactionTypeDebit) + "'",
			"transaction_status = '" + string(types.TransactionStatusCompleted) + "'",
			"transaction_reason = '" + string(types.TransactionReasonInvoicePayment) + "'",
		},
		Dimensions: map[string]string{"customer_id": "customer_id", "currency": "currency"},
		Shapes:     []types.Shape{types.ShapeScalar, types.ShapeTimeseries, types.ShapeBreakdown},
		Currency:   true,
	},
	types.MetricCreditTopups: {
		Metric:  types.MetricCreditTopups,
		Table:   "wallet_transactions",
		TimeCol: "created_at",
		Measure: "sum(amount)",
		Agg:     "sum",
		Arg:     "amount",
		Where: []string{
			"type = '" + string(types.TransactionTypeCredit) + "'",
			"transaction_status = '" + string(types.TransactionStatusCompleted) + "'",
			"transaction_reason IN (" + quotedList(creditTopupReasons) + ")",
		},
		Dimensions: map[string]string{
			"transaction_reason": "transaction_reason",
			"customer_id":        "customer_id",
			"currency":           "currency",
		},
		Shapes:   []types.Shape{types.ShapeScalar, types.ShapeTimeseries, types.ShapeBreakdown},
		Currency: true,
	},
	types.MetricInvoiceTotal:       invoiceTotalMetric(types.MetricInvoiceTotal, invoiceBillingTime("")),
	types.MetricInvoiceTotalIssued: invoiceTotalMetric(types.MetricInvoiceTotalIssued, "issue_date"),
	types.MetricNetBilling:         netBillingMetric(types.MetricNetBilling, invoiceBillingTime),
	types.MetricNetBillingIssued:   netBillingMetric(types.MetricNetBillingIssued, invoiceIssueTime),
	types.MetricActiveCustomers: {
		Metric:     types.MetricActiveCustomers,
		Dimensions: map[string]string{"currency": "currency"},
		Shapes:     []types.Shape{types.ShapeScalar},
		Currency:   true,
		Build:      buildActiveCustomers,
	},
	types.MetricEventsVolume: eventsVolumeMetric,
}

func LookupBillingMetric(m types.Metric) (*BillingMetric, bool) {
	bm, ok := billingMetrics[m]
	return bm, ok
}
