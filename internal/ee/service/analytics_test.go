package service

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/analytics"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/meter"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type AnalyticsServiceSuite struct {
	testutil.BaseServiceTestSuite
	svc            AnalyticsService
	meterUsageRepo *testutil.InMemoryMeterUsageStore

	now        time.Time
	rangeStart time.Time
	rangeEnd   time.Time
	sumMeter   *meter.Meter
	maxMeter   *meter.Meter
}

func TestAnalyticsService(t *testing.T) { suite.Run(t, new(AnalyticsServiceSuite)) }

func (s *AnalyticsServiceSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()

	s.meterUsageRepo = s.GetStores().MeterUsageRepo.(*testutil.InMemoryMeterUsageStore)

	s.now = time.Now().UTC()
	s.rangeStart = s.now.Add(-48 * time.Hour)
	s.rangeEnd = s.now.Add(24 * time.Hour)

	ctx := s.GetContext()

	s.sumMeter = &meter.Meter{
		ID:        "meter_sum",
		Name:      "SUM meter",
		EventName: "api_call",
		Aggregation: meter.Aggregation{
			Type: types.AggregationSum,
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().MeterRepo.CreateMeter(ctx, s.sumMeter))

	s.maxMeter = &meter.Meter{
		ID:        "meter_max",
		Name:      "MAX meter",
		EventName: "gauge",
		Aggregation: meter.Aggregation{
			Type: types.AggregationMax,
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().MeterRepo.CreateMeter(ctx, s.maxMeter))

	s.svc = NewAnalyticsService(ServiceParams{
		Logger:                 s.GetLogger(),
		Config:                 s.GetConfig(),
		DB:                     s.GetDB(),
		MeterUsageRepo:         s.meterUsageRepo,
		MeterRepo:              s.GetStores().MeterRepo,
		CustomerRepo:           s.GetStores().CustomerRepo,
		FeatureRepo:            s.GetStores().FeatureRepo,
		SubRepo:                s.GetStores().SubscriptionRepo,
		PriceRepo:              s.GetStores().PriceRepo,
		AddonRepo:              s.GetStores().AddonRepo,
		GroupRepo:              s.GetStores().GroupRepo,
		AnalyticsSavedViewRepo: s.GetStores().AnalyticsSavedViewRepo,
	})
}

func (s *AnalyticsServiceSuite) TearDownTest() {
	s.BaseServiceTestSuite.TearDownTest()
	s.meterUsageRepo.Clear()
}

// insertUsage inserts one meter_usage row with the given properties.
func (s *AnalyticsServiceSuite) insertUsage(ctx context.Context, meterID string, ts time.Time, qty float64, props map[string]interface{}) {
	s.NoError(s.meterUsageRepo.BulkInsertMeterUsage(ctx, []*events.MeterUsage{
		{
			Event: events.Event{
				ID:            types.GenerateUUID(),
				TenantID:      types.GetTenantID(ctx),
				EnvironmentID: types.GetEnvironmentID(ctx),
				Timestamp:     ts,
				EventName:     "api_call",
				Properties:    props,
			},
			MeterID:  meterID,
			QtyTotal: decimal.NewFromFloat(qty),
		},
	}))
}

func (s *AnalyticsServiceSuite) drVars() map[string]any {
	return map[string]any{
		"dr": map[string]any{
			"from": s.rangeStart.Format("2006-01-02"),
			"to":   s.rangeEnd.Format("2006-01-02"),
		},
	}
}

func (s *AnalyticsServiceSuite) breakdownDef() analytics.ViewDefinition {
	return analytics.ViewDefinition{
		Shape:      analytics.ShapeBreakdown,
		Metrics:    []string{"usage_quantity"},
		Dimensions: []string{"properties.region"},
		Filters:    []analytics.Filter{{Field: "meter_id", Op: "eq", Value: "{{meter}}"}},
		Time:       analytics.TimeSpecRaw{Range: "{{dr}}", Grain: "day"},
		Variables: []analytics.Variable{
			{Name: "meter", Type: "string", Required: true},
			{Name: "dr", Type: "date_range", Required: true},
		},
	}
}

// ---------------------------------------------------------------------------
// ExecuteView — breakdown
// ---------------------------------------------------------------------------

func (s *AnalyticsServiceSuite) TestExecuteView_BreakdownReturnsRows() {
	ctx := s.GetContext()
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 3, map[string]interface{}{"region": "us"})
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 4, map[string]interface{}{"region": "us"})
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 5, map[string]interface{}{"region": "eu"})

	vars := s.drVars()
	vars["meter"] = s.sumMeter.ID

	res, err := s.svc.ExecuteView(ctx, s.breakdownDef(), vars)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Require().Len(res.Columns, 2)
	s.Equal("properties.region", res.Columns[0].Name)
	s.Equal("dimension", res.Columns[0].Role)
	s.Equal("usage_quantity", res.Columns[1].Name)
	s.Equal("metric", res.Columns[1].Role)

	s.Require().Len(res.Rows, 2)
	totals := map[string]string{}
	for _, row := range res.Rows {
		s.Require().Len(row, 2)
		region, _ := row[0].(string)
		usage, _ := row[1].(string)
		totals[region] = usage
	}
	s.Equal("7", totals["us"])
	s.Equal("5", totals["eu"])
}

func (s *AnalyticsServiceSuite) TestExecuteView_BreakdownRejectsNonPropertyDimension() {
	ctx := s.GetContext()
	def := s.breakdownDef()
	def.Dimensions = []string{"meter_id"} // structural, not properties.* — Ruling C

	vars := s.drVars()
	vars["meter"] = s.sumMeter.ID

	_, err := s.svc.ExecuteView(ctx, def, vars)
	s.Error(err)
	s.True(ierr.IsValidation(err), "expected a validation error, got: %v", err)
}

// ---------------------------------------------------------------------------
// ExecuteView — timeseries
// ---------------------------------------------------------------------------

func (s *AnalyticsServiceSuite) timeseriesDef(meterID string) analytics.ViewDefinition {
	return analytics.ViewDefinition{
		Shape:   analytics.ShapeTimeseries,
		Metrics: []string{"usage_quantity"},
		Filters: []analytics.Filter{{Field: "meter_id", Op: "eq", Value: meterID}},
		Time:    analytics.TimeSpecRaw{Range: "{{dr}}", Grain: "day"},
		Variables: []analytics.Variable{
			{Name: "dr", Type: "date_range", Required: true},
		},
	}
}

// TestExecuteView_TimeseriesUsesMeterAggregation proves Ruling A: a MAX meter's
// timeseries execution must apply MAX (not silently default to SUM).
func (s *AnalyticsServiceSuite) TestExecuteView_TimeseriesUsesMeterAggregation() {
	ctx := s.GetContext()
	s.insertUsage(ctx, s.maxMeter.ID, s.now, 5, nil)
	s.insertUsage(ctx, s.maxMeter.ID, s.now, 20, nil)
	s.insertUsage(ctx, s.maxMeter.ID, s.now, 10, nil)

	res, err := s.svc.ExecuteView(ctx, s.timeseriesDef(s.maxMeter.ID), s.drVars())
	s.NoError(err)
	s.Require().NotNil(res)
	s.Require().Len(res.Columns, 2)
	s.Equal("window_start", res.Columns[0].Name)
	s.Equal("usage_quantity", res.Columns[1].Name)

	// MAX(5, 20, 10) == 20. If AggregationType had silently defaulted to SUM
	// (the bug Ruling A guards against), this would be 35 instead.
	s.Equal("20", res.Meta["total"])
	s.Require().Len(res.Rows, 1)
	s.Equal("20", res.Rows[0][1])
}

func (s *AnalyticsServiceSuite) TestExecuteView_TimeseriesRequiresExactlyOneMeter() {
	ctx := s.GetContext()

	def := s.timeseriesDef(s.maxMeter.ID)
	def.Filters = nil // no meter_id filter at all

	_, err := s.svc.ExecuteView(ctx, def, s.drVars())
	s.Error(err)
	s.True(ierr.IsValidation(err), "expected a validation error, got: %v", err)
}

// ---------------------------------------------------------------------------
// Saved-view CRUD
// ---------------------------------------------------------------------------

func (s *AnalyticsServiceSuite) TestCreateView_QuerySavedView_RoundTrip() {
	ctx := s.GetContext()
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 3, map[string]interface{}{"region": "us"})
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 5, map[string]interface{}{"region": "eu"})

	view := &analytics.SavedView{
		Name:       "region breakdown",
		Definition: s.breakdownDef(),
	}
	s.NoError(s.svc.CreateView(ctx, view))
	s.NotEmpty(view.ID, "CreateView should assign an ID")

	vars := s.drVars()
	vars["meter"] = s.sumMeter.ID

	direct, err := s.svc.ExecuteView(ctx, s.breakdownDef(), vars)
	s.NoError(err)

	viaSaved, err := s.svc.QuerySavedView(ctx, view.ID, vars)
	s.NoError(err)
	s.Equal(direct, viaSaved)
}

// TestCreateView_DefaultsVersionToOne proves the Version invariant now lives in
// the service: a caller that leaves Version unset (zero value) still gets a
// persisted saved view with Version == 1, regardless of what the handler does.
func (s *AnalyticsServiceSuite) TestCreateView_DefaultsVersionToOne() {
	ctx := s.GetContext()
	view := &analytics.SavedView{
		Name:       "no version set",
		Definition: s.breakdownDef(),
	}
	s.Equal(0, view.Version, "precondition: caller left Version unset")

	s.NoError(s.svc.CreateView(ctx, view))
	s.Equal(1, view.Version, "CreateView should default Version to 1 on the in-memory value")

	stored, err := s.GetStores().AnalyticsSavedViewRepo.Get(ctx, view.ID)
	s.NoError(err)
	s.Equal(1, stored.Version, "CreateView should persist Version == 1")
}

func (s *AnalyticsServiceSuite) TestCreateView_RejectsInvalidDefinition() {
	ctx := s.GetContext()
	view := &analytics.SavedView{
		Name:       "invalid",
		Definition: analytics.ViewDefinition{Shape: analytics.ShapeBreakdown}, // no metrics
	}
	err := s.svc.CreateView(ctx, view)
	s.Error(err)
	s.True(ierr.IsValidation(err))
}

func (s *AnalyticsServiceSuite) TestQuerySavedView_CrossTenantDoesNotLeak() {
	ctx := s.GetContext()
	view := &analytics.SavedView{
		Name:       "tenant-a view",
		Definition: s.breakdownDef(),
	}
	s.NoError(s.svc.CreateView(ctx, view))

	otherTenantCtx := types.SetTenantID(ctx, "other_tenant_xyz")

	vars := s.drVars()
	vars["meter"] = s.sumMeter.ID

	_, err := s.svc.QuerySavedView(otherTenantCtx, view.ID, vars)
	s.Error(err)
	s.True(ierr.IsNotFound(err), "expected not-found error for cross-tenant access, got: %v", err)
}
