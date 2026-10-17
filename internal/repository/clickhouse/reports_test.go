package clickhouse

import (
	"context"
	"slices"
	"testing"

	"github.com/flexprice/flexprice/internal/domain/analytics"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingSQL_InvoiceTotalUsesConstants(t *testing.T) {
	_, sq, ok := lookupBillingMetric(types.MetricInvoiceTotal)
	require.True(t, ok)
	assert.Equal(t, "invoices", sq.Table)
	assert.Contains(t, sq.Where, "invoice_status = '"+string(types.InvoiceStatusFinalized)+"'")
}

func TestBillingSQL_MatchesDomainRegistry(t *testing.T) {
	metrics := analytics.BillingMetrics()
	assert.Len(t, billingSQLs, len(metrics))
	for _, m := range metrics {
		bm, sq, ok := lookupBillingMetric(m)
		require.True(t, ok, "metric %q has no SQL", m)
		for d := range sq.DimExprs {
			assert.True(t, slices.Contains(bm.Dimensions, d), "metric %q: unknown dimension %q", m, d)
		}
	}
}

func TestReportResult_OverflowIsValidationError(t *testing.T) {
	q, err := buildBillingQuery(billingRV(types.ShapeScalar), "reports_db", "t", "e", 2)
	require.NoError(t, err)
	_, err = reportResult(q, [][]string{{"usd", "1"}, {"inr", "2"}, {"eur", "3"}}, 2)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))
}

func TestReportResult_TopNSkipsOverflow(t *testing.T) {
	rv := billingRV(types.ShapeBreakdown, "customer_id")
	rv.Limit = 2
	q, err := buildBillingQuery(rv, "reports_db", "t", "e", 2)
	require.NoError(t, err)
	res, err := reportResult(q, [][]string{{"c1", "usd", "50"}, {"c2", "usd", "20"}, {"c3", "usd", "5"}}, 2)
	require.NoError(t, err)
	assert.Len(t, res.Rows, 3)
	assert.Equal(t, q.Columns, res.Columns)
}

func TestRunReport_DisabledIs403(t *testing.T) {
	_, err := (&ReportsRepository{}).RunReport(context.Background(), billingRV(types.ShapeScalar), "t", "e")
	require.Error(t, err)
	assert.True(t, ierr.IsPermissionDenied(err))
}
