package analytics

import (
	"testing"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customRV(metric types.Metric, shape types.Shape) *ResolvedView {
	rv := billingRV(shape)
	rv.Metrics = []types.Metric{metric}
	return rv
}

func TestBuildNetBilling(t *testing.T) {
	q, err := BuildBillingQuery(customRV(types.MetricNetBilling, types.ShapeTimeseries), "reports_db", "t", "e", 10)
	require.NoError(t, err)
	for _, alias := range []string{"i", "n"} {
		assert.Contains(t, q.SQL, alias+".tenant_id = {tenant_id:String} AND "+alias+".environment_id = {environment_id:String} AND "+alias+"._peerdb_is_deleted = 0 AND "+alias+".status = 'published'")
	}
	assert.Contains(t, q.SQL, "reports_db.invoices AS i FINAL")
	assert.Contains(t, q.SQL, "reports_db.credit_notes AS n FINAL")
	assert.Contains(t, q.SQL, "i.invoice_status = 'FINALIZED'")
	assert.Contains(t, q.SQL, "n.credit_note_status = 'FINALIZED'")
	assert.Contains(t, q.SQL, "nullIf(coalesce(nullIf(i.period_start, toDateTime64(0, 6)), i.issue_date), toDateTime64(0, 6)) >= toDateTime64({from:String}, 6, 'UTC')")
	assert.Contains(t, q.SQL, "SELECT nullIf(coalesce(nullIf(period_start, toDateTime64(0, 6)), issue_date), toDateTime64(0, 6)) AS b")
	assert.NotContains(t, q.SQL, "i.finalized_at")
	assert.Contains(t, q.SQL, "nullIf(n.created_at, toDateTime64(0, 6)) >= toDateTime64({from:String}, 6, 'UTC')")
	assert.Contains(t, q.SQL, "GROUP BY bucket, d_currency ORDER BY bucket, d_currency LIMIT 11")
	assert.NotContains(t, q.SQL, "'t'")
	require.Len(t, q.Columns, 3)
	assert.Equal(t, "net_billing", q.Columns[2].Name)

	q, err = BuildBillingQuery(customRV(types.MetricNetBilling, types.ShapeScalar), "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Equal(t, 2, len(q.Columns))
	assert.NotContains(t, q.SQL, "LIMIT")
}

func TestBuildNetBilling_Rejects(t *testing.T) {
	rv := customRV(types.MetricNetBilling, types.ShapeScalar)
	rv.Compare = "previous_period"
	_, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))

	_, err = BuildBillingQuery(customRV(types.MetricNetBilling, types.ShapeBreakdown), "reports_db", "t", "e", 0)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))

	rv = customRV(types.MetricNetBilling, types.ShapeScalar)
	rv.Dimensions = []string{"customer_id"}
	_, err = BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))
}

func currencyFilter() []*Filter {
	return []*Filter{{Field: "currency", Op: types.EQUAL, Value: []string{"usd", "eur"}}}
}

func TestBuildNetBilling_CurrencyFilter(t *testing.T) {
	rv := customRV(types.MetricNetBilling, types.ShapeScalar)
	rv.Filters = currencyFilter()
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)
	for _, alias := range []string{"i", "n"} {
		assert.Contains(t, q.SQL, alias+".currency IN ({f0_0:String}, {f0_1:String})")
	}
	assert.Equal(t, "usd", q.Params["f0_0"])
	assert.Equal(t, "eur", q.Params["f0_1"])

	rv.Filters = []*Filter{{Field: "customer_id", Op: types.EQUAL, Value: []string{"c"}}}
	_, err = BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))
}

func TestBuildActiveCustomers_CurrencyFilter(t *testing.T) {
	rv := customRV(types.MetricActiveCustomers, types.ShapeScalar)
	rv.Filters = currencyFilter()
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "s.currency IN ({f0_0:String}, {f0_1:String})")
	assert.Equal(t, "usd", q.Params["f0_0"])
	assert.Equal(t, "eur", q.Params["f0_1"])

	rv.Filters = []*Filter{{Field: "customer_id", Op: types.EQUAL, Value: []string{"c"}}}
	_, err = BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))
}

func TestBuildActiveCustomers(t *testing.T) {
	q, err := BuildBillingQuery(customRV(types.MetricActiveCustomers, types.ShapeScalar), "reports_db", "t", "e", 5)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "s.tenant_id = {tenant_id:String} AND s.environment_id = {environment_id:String} AND s._peerdb_is_deleted = 0 AND s.status = 'published'")
	assert.Contains(t, q.SQL, "reports_db.subscriptions AS s FINAL")
	assert.Contains(t, q.SQL, "s.subscription_status IN ('active', 'trialing', 'cancelled')")
	assert.Contains(t, q.SQL, "s.subscription_status != 'cancelled' OR coalesce(nullIf(s.cancelled_at")
	assert.Contains(t, q.SQL, "s.start_date < toDateTime64({to:String}, 6, 'UTC')")
	assert.Contains(t, q.SQL, "> toDateTime64({from:String}, 6, 'UTC')")
	assert.Contains(t, q.SQL, "LIMIT 6")
	assert.Equal(t, "active_customers", q.Columns[1].Name)

	for _, mut := range []func(*ResolvedView){
		func(rv *ResolvedView) { rv.Compare = "previous_year" },
		func(rv *ResolvedView) { rv.Shape = types.ShapeTimeseries },
		func(rv *ResolvedView) { rv.Shape = types.ShapeBreakdown },
	} {
		rv := customRV(types.MetricActiveCustomers, types.ShapeScalar)
		mut(rv)
		_, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
		require.Error(t, err)
		assert.True(t, ierr.IsValidation(err))
	}
}
