package service

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/domain/analytics"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func billingCtx() context.Context {
	ctx := context.WithValue(context.Background(), types.CtxTenantID, "tenant_1")
	return context.WithValue(ctx, types.CtxEnvironmentID, "env_1")
}

func newBillingSvc(repo *testutil.InMemoryReportsRepo) AnalyticsService {
	if repo.Max == 0 {
		repo.Max = 500
	}
	return NewAnalyticsService(ServiceParams{ReportsRepo: repo})
}

func billingView(shape types.Shape, dims ...string) *analytics.ViewDefinition {
	return &analytics.ViewDefinition{
		Family:     types.FamilyBilling,
		Shape:      shape,
		Metrics:    []types.Metric{types.MetricInvoiceTotal},
		Dimensions: dims,
		Time:       analytics.TimeSpecRaw{Range: "last_30_days", Grain: types.GrainMonth},
	}
}

func TestExecuteView_BillingDisabledIs403(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: false}
	_, err := newBillingSvc(repo).ExecuteView(billingCtx(), billingView(types.ShapeScalar), nil)
	require.Error(t, err)
	assert.True(t, ierr.IsPermissionDenied(err))
	assert.Nil(t, repo.Last)
}

func TestExecuteView_BillingScopesFromContext(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: true, DB: "reports_db", Rows: [][]string{{"usd", "10"}}}
	res, err := newBillingSvc(repo).ExecuteView(billingCtx(), billingView(types.ShapeScalar), nil)
	require.NoError(t, err)
	require.NotNil(t, repo.Last)
	assert.Equal(t, "tenant_1", repo.Last.Params["tenant_id"])
	assert.Equal(t, "env_1", repo.Last.Params["environment_id"])
	assert.Contains(t, repo.Last.SQL, "LIMIT 501")
	assert.Equal(t, "billing", res.Meta.QuerySource)
	require.Len(t, res.Columns, 2)
	assert.Equal(t, "currency", res.Columns[0].Name)
	assert.Equal(t, "", res.Columns[1].Currency)
	assert.Equal(t, [][]string{{"usd", "10"}}, res.Rows)
}

func TestExecuteView_BillingMissingTenantIs403(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: true, DB: "reports_db"}
	_, err := newBillingSvc(repo).ExecuteView(context.Background(), billingView(types.ShapeScalar), nil)
	require.Error(t, err)
	assert.True(t, ierr.IsPermissionDenied(err))
	assert.Nil(t, repo.Last)
}

func TestExecuteView_BillingBreakdownSortAndLimit(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: true, DB: "reports_db", Rows: [][]string{
		{"c1", "usd", "5"}, {"c2", "usd", "50"}, {"c3", "usd", "20"},
	}}
	v := billingView(types.ShapeBreakdown, "customer_id")
	v.Sort = []*analytics.SortSpec{{Field: "invoice_total", Dir: types.SortDirectionDesc}}
	v.Limit = 2
	res, err := newBillingSvc(repo).ExecuteView(billingCtx(), v, nil)
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"c2", "usd", "50"}, {"c3", "usd", "20"}}, res.Rows)
}

func TestExecuteView_BillingOverflowIsValidationError(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: true, DB: "reports_db", Max: 2, Rows: [][]string{{"usd", "1"}, {"inr", "2"}, {"eur", "3"}}}
	_, err := newBillingSvc(repo).ExecuteView(billingCtx(), billingView(types.ShapeScalar), nil)
	require.Error(t, err)
	assert.True(t, ierr.IsValidation(err))
}

func TestExecuteView_BillingRunErrorWithNilLogger(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: true, DB: "reports_db", Err: ierr.NewError("boom").Mark(ierr.ErrDatabase)}
	_, err := newBillingSvc(repo).ExecuteView(billingCtx(), billingView(types.ShapeScalar), nil)
	require.Error(t, err)
}

func TestExecuteView_BillingEmptyRowsNotNil(t *testing.T) {
	repo := &testutil.InMemoryReportsRepo{On: true, DB: "reports_db"}
	res, err := newBillingSvc(repo).ExecuteView(billingCtx(), billingView(types.ShapeScalar), nil)
	require.NoError(t, err)
	assert.NotNil(t, res.Rows)
}
