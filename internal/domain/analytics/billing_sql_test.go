package analytics

import (
	"strings"
	"testing"
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const billTime = "nullIf(coalesce(nullIf(period_start, toDateTime64(0, 6)), issue_date), toDateTime64(0, 6))"

func billingRV(shape types.Shape, dims ...string) *ResolvedView {
	return &ResolvedView{
		Family:     types.FamilyBilling,
		Shape:      shape,
		Metrics:    []types.Metric{types.MetricInvoiceTotal},
		Dimensions: dims,
		Time: TimeSpec{
			From:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			To:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
			Grain: types.GrainMonth,
		},
	}
}

func TestBuildBillingQuery_ScopesAndInvariants(t *testing.T) {
	q, err := BuildBillingQuery(billingRV(types.ShapeScalar), "reports_db", "tenant_1", "env_1", 1000)
	require.NoError(t, err)
	for _, frag := range []string{
		"FROM reports_db.invoices FINAL",
		"tenant_id = {tenant_id:String}",
		"environment_id = {environment_id:String}",
		"_peerdb_is_deleted = 0",
		"status = 'published'",
		"invoice_status = 'FINALIZED'",
		billTime + " >= toDateTime64({from:String}, 6, 'UTC')",
		billTime + " < toDateTime64({to:String}, 6, 'UTC')",
		"LIMIT 1001",
	} {
		assert.Contains(t, q.SQL, frag)
	}
	assert.Equal(t, "tenant_1", q.Params["tenant_id"])
	assert.Equal(t, "env_1", q.Params["environment_id"])
	assert.Equal(t, "2026-09-01 00:00:00.000000", q.Params["from"])
	assert.Equal(t, "2026-10-01 00:00:00.000000", q.Params["to"])
}

func TestBuildBillingQuery_ScalarForcesCurrency(t *testing.T) {
	q, err := BuildBillingQuery(billingRV(types.ShapeScalar), "reports_db", "t", "e", 0)
	require.NoError(t, err)
	require.Len(t, q.Columns, 2)
	assert.Equal(t, ReportColumn{Name: "currency", Type: types.ColumnTypeString, Role: types.ColumnRoleDimension}, q.Columns[0])
	assert.Equal(t, ReportColumn{Name: "invoice_total", Type: types.ColumnTypeDecimal, Role: types.ColumnRoleMetric}, q.Columns[1])
	assert.Contains(t, q.SQL, "GROUP BY d_currency ORDER BY d_currency")
	assert.NotContains(t, q.SQL, "LIMIT")
}

func TestBuildBillingQuery_TimeseriesBucketsInUTC(t *testing.T) {
	q, err := BuildBillingQuery(billingRV(types.ShapeTimeseries, "customer_id"), "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "toStartOfMonth("+billTime+", 'UTC')")
	assert.Equal(t, []string{"bucket", "customer_id", "currency", "invoice_total"}, columnNames(q))
	assert.Contains(t, q.SQL, "GROUP BY bucket, d_customer_id, d_currency")
	assert.Contains(t, q.SQL, "ORDER BY bucket, d_customer_id, d_currency")
}

func TestBuildBillingQuery_GrainMap(t *testing.T) {
	for grain, fn := range map[types.Grain]string{
		"": "toStartOfDay", types.GrainDay: "toStartOfDay", types.GrainWeek: "toMonday", types.GrainMonth: "toStartOfMonth",
	} {
		rv := billingRV(types.ShapeTimeseries)
		rv.Time.Grain = grain
		q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
		require.NoError(t, err)
		assert.Contains(t, q.SQL, fn+"(nullIf(", "grain %q", grain)
	}
}

func TestBuildBillingQuery_BreakdownOrdersByMeasure(t *testing.T) {
	q, err := BuildBillingQuery(billingRV(types.ShapeBreakdown, "currency"), "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"currency", "invoice_total"}, columnNames(q))
	assert.Contains(t, q.SQL, "ORDER BY sum(total) DESC")
}

func TestBuildBillingQuery_TopNPushesLimit(t *testing.T) {
	rv := billingRV(types.ShapeBreakdown, "customer_id")
	rv.Limit = 10
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 1000)
	require.NoError(t, err)
	assert.True(t, q.TopN)
	assert.Contains(t, q.SQL, "ORDER BY sum(total) DESC LIMIT 10")
	assert.NotContains(t, q.SQL, "LIMIT 1001")

	rv.Sort = []*SortSpec{{Field: "invoice_total", Dir: types.SortDirectionDesc}}
	q, err = BuildBillingQuery(rv, "reports_db", "t", "e", 1000)
	require.NoError(t, err)
	assert.True(t, q.TopN)

	rv.Limit = 5000
	q, err = BuildBillingQuery(rv, "reports_db", "t", "e", 1000)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "LIMIT 1000")
}

func TestBuildBillingQuery_NonTopNKeepsOverflowProbe(t *testing.T) {
	rv := billingRV(types.ShapeBreakdown, "customer_id")
	rv.Limit = 10
	rv.Sort = []*SortSpec{{Field: "customer_id", Dir: types.SortDirectionAsc}}
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 1000)
	require.NoError(t, err)
	assert.False(t, q.TopN)
	assert.Contains(t, q.SQL, "LIMIT 1001")

	rv.Sort = []*SortSpec{{Field: "invoice_total"}}
	q, err = BuildBillingQuery(rv, "reports_db", "t", "e", 1000)
	require.NoError(t, err)
	assert.False(t, q.TopN)
}

func TestBuildBillingQuery_FilterValuesAreParams(t *testing.T) {
	rv := billingRV(types.ShapeScalar)
	evil := "x' OR 1=1 --"
	rv.Filters = []*Filter{{Field: "customer_id", Op: types.IN, Value: []string{"cust_1", evil}}}
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "customer_id IN ({f0_0:String}, {f0_1:String})")
	assert.NotContains(t, q.SQL, "OR 1=1")
	assert.Equal(t, "cust_1", q.Params["f0_0"])
	assert.Equal(t, evil, q.Params["f0_1"])
}

func TestBuildBillingQuery_EmptyFilterMatchesNothing(t *testing.T) {
	rv := billingRV(types.ShapeScalar)
	rv.Filters = []*Filter{{Field: "customer_id", Op: types.IN, Value: nil}}
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "1 = 0")
}

func TestBuildBillingQuery_RequiresTenantAndEnvironment(t *testing.T) {
	for _, te := range [][2]string{{"", "e"}, {"t", ""}} {
		_, err := BuildBillingQuery(billingRV(types.ShapeScalar), "reports_db", te[0], te[1], 0)
		require.Error(t, err)
		assert.True(t, ierr.IsPermissionDenied(err))
	}
}

func TestBuildBillingQuery_RejectsBadDatabaseIdentifier(t *testing.T) {
	for _, db := range []string{"", "a.b", "db; DROP", "1db"} {
		_, err := BuildBillingQuery(billingRV(types.ShapeScalar), db, "t", "e", 0)
		require.Error(t, err, "db %q", db)
	}
}

func TestBuildBillingQuery_RejectsNonAllowlistedInputs(t *testing.T) {
	rv := billingRV(types.ShapeBreakdown, "meter_id")
	_, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.Error(t, err)

	rv = billingRV(types.ShapeScalar)
	rv.Filters = []*Filter{{Field: "tenant_id", Op: types.EQUAL, Value: []string{"other"}}}
	_, err = BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.Error(t, err)
}

func TestBuildBillingQuery_SelectsAreStringSafe(t *testing.T) {
	q, err := BuildBillingQuery(billingRV(types.ShapeTimeseries, "customer_id"), "reports_db", "t", "e", 0)
	require.NoError(t, err)
	selectPart := q.SQL[:strings.Index(q.SQL, " FROM ")]
	assert.Equal(t, len(q.Columns), strings.Count(selectPart, "ifNull("))
	assert.Contains(t, selectPart, "'%Y-%m-%dT%H:%i:%SZ'")
}

func columnNames(q *ReportQuery) []string {
	out := make([]string, len(q.Columns))
	for i, c := range q.Columns {
		out[i] = c.Name
	}
	return out
}

func TestPreviousWindow(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		name        string
		from, to    time.Time
		compare     string
		pf, pt      time.Time
		shiftMonths int
	}{
		{"full month", from, to, "previous_period", d(2026, 8, 1), d(2026, 9, 1), 1},
		{"month to date", d(2026, 10, 1), d(2026, 10, 13), "previous_period", d(2026, 9, 1), d(2026, 9, 13), 1},
		{"quarter", d(2026, 7, 1), d(2026, 10, 1), "previous_period", d(2026, 4, 1), d(2026, 7, 1), 3},
		{"year to date", d(2026, 1, 1), d(2026, 10, 13), "previous_period", d(2025, 3, 1), d(2025, 12, 13), 10},
		{"rolling", d(2026, 9, 5), d(2026, 9, 12), "previous_period", d(2026, 8, 29), d(2026, 9, 5), 0},
		{"year", from, to, "previous_year", d(2025, 9, 1), d(2025, 10, 1), 12},
		{"leap", d(2028, 2, 29), d(2028, 3, 1), "previous_year", d(2027, 3, 1), d(2027, 3, 1), 12},
		{"mar full", d(2026, 3, 1), d(2026, 3, 31), "previous_period", d(2026, 2, 1), d(2026, 3, 1), 1},
		{"q1 to mar 31", d(2026, 1, 1), d(2026, 3, 31), "previous_period", d(2025, 10, 1), d(2025, 12, 31), 3},
		{"may full", d(2026, 5, 1), d(2026, 5, 31), "previous_period", d(2026, 4, 1), d(2026, 5, 1), 1},
		{"oct full", d(2026, 10, 1), d(2026, 10, 31), "previous_period", d(2026, 9, 1), d(2026, 10, 1), 1},
	}
	for _, c := range cases {
		pf, pt, shift, err := PreviousWindow(c.from, c.to, c.compare)
		require.NoError(t, err, c.name)
		assert.Equal(t, c.pf, pf, c.name)
		assert.Equal(t, c.pt, pt, c.name)
		assert.Equal(t, c.shiftMonths, shift, c.name)
	}

	_, _, _, err := PreviousWindow(from, to, "bogus")
	require.Error(t, err)
}

func TestPreviousWindow_NeverOverlaps(t *testing.T) {
	for m := time.January; m <= time.December; m++ {
		from := time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)
		for day := 2; day <= 31; day++ {
			to := time.Date(2026, m, day, 0, 0, 0, 0, time.UTC)
			if to.Month() != m {
				continue
			}
			for _, cmp := range []string{"previous_period", "previous_year"} {
				pf, pt, _, err := PreviousWindow(from, to, cmp)
				require.NoError(t, err)
				assert.False(t, pt.After(from), "%s %s..%s -> %s..%s", cmp, from, to, pf, pt)
				assert.True(t, pf.Before(pt), "%s %s..%s", cmp, from, to)
			}
		}
	}
}

func TestBuildBillingQuery_CompareTimeseries(t *testing.T) {
	rv := billingRV(types.ShapeTimeseries)
	rv.Compare = "previous_period"
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"bucket", "currency", "invoice_total", "invoice_total_prev"}, columnNames(q))
	assert.Contains(t, q.SQL, "sumIf(total, ")
	assert.Contains(t, q.SQL, "addMonths("+billTime+", {shift_m:Int32})")
	assert.Contains(t, q.SQL, "toDateTime64({prev_from:String}, 6, 'UTC')")
	tc := billTime
	cur := tc + " >= toDateTime64({from:String}, 6, 'UTC') AND " + tc + " < toDateTime64({to:String}, 6, 'UTC')"
	prev := tc + " >= toDateTime64({prev_from:String}, 6, 'UTC') AND " + tc + " < toDateTime64({prev_to:String}, 6, 'UTC')"
	assert.Contains(t, q.SQL, " AND (("+cur+") OR ("+prev+"))")
	assert.Equal(t, "2026-08-01 00:00:00.000000", q.Params["prev_from"])
	assert.Equal(t, "2026-09-01 00:00:00.000000", q.Params["prev_to"])
	assert.Equal(t, "1", q.Params["shift_m"])
	_, hasShift := q.Params["shift_s"]
	assert.False(t, hasShift)
}

func TestBuildBillingQuery_CompareRollingUsesSeconds(t *testing.T) {
	rv := billingRV(types.ShapeTimeseries)
	rv.Time.From = time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	rv.Time.To = time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	rv.Compare = "previous_period"
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "addSeconds("+billTime+", {shift_s:Int64})")
	assert.Equal(t, "604800", q.Params["shift_s"])
	_, hasShift := q.Params["shift_m"]
	assert.False(t, hasShift)
}

func TestBuildBillingQuery_CompareYearUsesAddYears(t *testing.T) {
	rv := billingRV(types.ShapeTimeseries)
	rv.Compare = "previous_year"
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "addYears("+billTime+", 1)")
	_, hasShift := q.Params["shift_s"]
	assert.False(t, hasShift)
}

func TestBuildBillingQuery_CompareTopNOrdersByCurrent(t *testing.T) {
	rv := billingRV(types.ShapeBreakdown, "customer_id")
	rv.Compare = "previous_period"
	rv.Limit = 5
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 1000)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "ORDER BY sumIf(total, ")
	assert.Contains(t, q.SQL, " DESC LIMIT 5")
	assert.True(t, q.TopN)
}

func TestBuildBillingQuery_NoCompareUnchanged(t *testing.T) {
	q, err := BuildBillingQuery(billingRV(types.ShapeScalar), "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.NotContains(t, q.SQL, "sumIf(")
	_, ok := q.Params["prev_from"]
	assert.False(t, ok)
}

func TestBuildBillingQuery_RejectsBadCompare(t *testing.T) {
	rv := billingRV(types.ShapeScalar)
	rv.Compare = "last_decade"
	_, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))
}

func TestBuildBillingQuery_SnapshotHasNoTimeWindow(t *testing.T) {
	withMetric(t, &BillingMetric{
		Metric: "test_snapshot", Table: "invoices", TimeCol: "finalized_at", Measure: "sum(amount_remaining)", Agg: "sum", Arg: "amount_remaining",
		Dimensions: map[string]string{"currency": "currency"}, Shapes: []types.Shape{types.ShapeScalar, types.ShapeBreakdown}, Currency: true, Snapshot: true,
	})
	rv := billingRV(types.ShapeScalar)
	rv.Metrics = []types.Metric{"test_snapshot"}
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.NotContains(t, q.SQL, "{from:String}")
	assert.NotContains(t, q.SQL, "{to:String}")

	rv.Compare = "previous_period"
	_, err = BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))
}

func TestBuildBillingQuery_ExpressionDimension(t *testing.T) {
	withMetric(t, &BillingMetric{
		Metric: "test_expr", Table: "invoices", TimeCol: "finalized_at", Measure: "sum(total)", Agg: "sum", Arg: "total",
		Dimensions: map[string]string{"bucket_x": "multiIf(total > 100, 'big', 'small')", "currency": "currency"},
		Shapes:     []types.Shape{types.ShapeBreakdown}, Currency: true,
	})
	rv := billingRV(types.ShapeBreakdown, "bucket_x")
	rv.Metrics = []types.Metric{"test_expr"}
	rv.Filters = []*Filter{{Field: "bucket_x", Op: types.EQUAL, Value: []string{"big"}}}
	q, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)
	assert.Contains(t, q.SQL, "ifNull(toString(multiIf(total > 100, 'big', 'small')), '') AS d_bucket_x")
	assert.Contains(t, q.SQL, "GROUP BY d_bucket_x")
	assert.Contains(t, q.SQL, "multiIf(total > 100, 'big', 'small') IN ({f0_0:String})")
}

func TestBuildBillingQuery_CustomBuildHook(t *testing.T) {
	var got BuildInput
	withMetric(t, &BillingMetric{
		Metric: "test_custom", Shapes: []types.Shape{types.ShapeScalar}, Currency: true,
		Build: func(in BuildInput) (*ReportQuery, error) {
			got = in
			return &ReportQuery{SQL: "SELECT 1 FROM " + in.DB + ".x i WHERE " + in.Scope("i") + " AND " + in.TimeCond("i.created_at"), Params: in.Params}, nil
		},
	})
	rv := billingRV(types.ShapeScalar)
	rv.Metrics = []types.Metric{"test_custom"}
	q, err := BuildBillingQuery(rv, "reports_db", "tenant_1", "env_1", 0)
	require.NoError(t, err)
	assert.Equal(t, "tenant_1", got.Params["tenant_id"])
	assert.Contains(t, q.SQL, "i.tenant_id = {tenant_id:String} AND i.environment_id = {environment_id:String} AND i._peerdb_is_deleted = 0 AND i.status = 'published'")
	assert.Contains(t, q.SQL, "nullIf(i.created_at, toDateTime64(0, 6)) >= toDateTime64({from:String}, 6, 'UTC')")

	_, err = BuildBillingQuery(rv, "reports_db", "", "env_1", 0)
	require.Error(t, err)
	assert.True(t, ierr.IsPermissionDenied(err))
}

func TestBuildBillingQuery_ARPURejectsCompare(t *testing.T) {
	rv := billingRV(types.ShapeScalar)
	rv.Metrics = []types.Metric{types.MetricARPU}
	_, err := BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.NoError(t, err)

	rv.Compare = "previous_period"
	_, err = BuildBillingQuery(rv, "reports_db", "t", "e", 0)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))
}
