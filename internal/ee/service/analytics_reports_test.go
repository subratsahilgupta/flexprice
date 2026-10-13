package service

import (
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reportsSvc(repo *testutil.InMemoryReportsRepo) AnalyticsService {
	if repo.Max == 0 {
		repo.Max = 500
	}
	return NewAnalyticsService(ServiceParams{ReportsRepo: repo})
}

func TestListReports(t *testing.T) {
	res, err := reportsSvc(&testutil.InMemoryReportsRepo{On: true, DB: "reports_db"}).ListReports(billingCtx())
	require.NoError(t, err)
	require.NotEmpty(t, res.Items)
	assert.Equal(t, "invoice_total", res.Items[0].Key)
	assert.Equal(t, "Total billing", res.Items[0].Name)
}

func TestListReports_DisabledIs403(t *testing.T) {
	_, err := reportsSvc(&testutil.InMemoryReportsRepo{On: false}).ListReports(billingCtx())
	require.Error(t, err)
	assert.True(t, ierr.IsPermissionDenied(err))
}

func TestRunReport_UnknownIs404(t *testing.T) {
	_, err := reportsSvc(&testutil.InMemoryReportsRepo{On: true, DB: "reports_db"}).RunReport(billingCtx(), "nope", &dto.ReportQueryRequest{Shape: "scalar", Range: "2026-09-01..2026-10-01"})
	require.Error(t, err)
	assert.True(t, ierr.IsNotFound(err))
}

func TestRunReport_RejectsDisallowed(t *testing.T) {
	svc := reportsSvc(&testutil.InMemoryReportsRepo{On: true, DB: "reports_db"})
	for _, req := range []*dto.ReportQueryRequest{
		{Shape: "histogram", Range: "2026-09-01..2026-10-01"},
		{Shape: "breakdown", Dimensions: []string{"meter_id"}, Range: "2026-09-01..2026-10-01"},
		{Shape: "timeseries", Grain: "hour", Range: "2026-09-01..2026-10-01"},
		{Shape: "scalar", Compare: "last_decade", Range: "2026-09-01..2026-10-01"},
		{Shape: "scalar", Range: "2026-09-01..2026-10-01", Filters: []dto.ReportFilter{{Field: "tenant_id", Op: "eq", Value: []string{"x"}}}},
	} {
		_, err := svc.RunReport(billingCtx(), "invoice_total", req)
		require.Error(t, err, "%+v", req)
		assert.True(t, ierr.IsValidation(err), "%+v", req)
	}
}

func TestRunReport_CompareAddsChangeAndMeta(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: true, DB: "reports_db", Rows: [][]string{{"usd", "110", "100"}, {"eur", "5", "0"}, {"inr", "x", "1"}}}
	res, err := reportsSvc(repo).RunReport(billingCtx(), "invoice_total", &dto.ReportQueryRequest{Shape: "scalar", Range: "2026-09-01..2026-10-01", Compare: "previous_period"})
	require.NoError(t, err)
	names := []string{}
	for _, c := range res.Columns {
		names = append(names, c.Name)
	}
	assert.Equal(t, []string{"currency", "invoice_total", "invoice_total_prev", "invoice_total_change_pct"}, names)
	assert.Equal(t, []string{"usd", "110", "100", "10"}, res.Rows[0])
	assert.Equal(t, "", res.Rows[1][3])
	assert.Equal(t, "", res.Rows[2][3])
	assert.Equal(t, "2026-09-01T00:00:00Z", res.Meta.From)
	assert.Equal(t, "2026-10-01T00:00:00Z", res.Meta.To)
	assert.Equal(t, "2026-08-02T00:00:00Z", res.Meta.PrevFrom)
	assert.Equal(t, "2026-09-01T00:00:00Z", res.Meta.PrevTo)
	assert.Equal(t, "2026-08-02 00:00:00.000000", repo.Last.Params["prev_from"])
}

func TestRunReport_CurrencyBecomesFilter(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: true, DB: "reports_db"}
	_, err := reportsSvc(repo).RunReport(billingCtx(), "invoice_total", &dto.ReportQueryRequest{Shape: "scalar", Range: "2026-09-01..2026-10-01", Currency: "usd"})
	require.NoError(t, err)
	assert.Contains(t, repo.Last.SQL, "currency IN ({f0_0:String})")
	assert.Equal(t, "usd", repo.Last.Params["f0_0"])
}

func TestRunReport_NoCompareNoMetaPrev(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: true, DB: "reports_db", Rows: [][]string{{"usd", "1"}}}
	res, err := reportsSvc(repo).RunReport(billingCtx(), "invoice_total", &dto.ReportQueryRequest{Shape: "scalar", Range: "2026-09-01..2026-10-01"})
	require.NoError(t, err)
	assert.Empty(t, res.Meta.PrevFrom)
	assert.Len(t, res.Columns, 2)
}
