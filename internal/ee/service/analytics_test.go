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
		Logger:            s.GetLogger(),
		Config:            s.GetConfig(),
		DB:                s.GetDB(),
		MeterUsageRepo:    s.meterUsageRepo,
		MeterRepo:         s.GetStores().MeterRepo,
		CustomerRepo:      s.GetStores().CustomerRepo,
		FeatureRepo:       s.GetStores().FeatureRepo,
		SubRepo:           s.GetStores().SubscriptionRepo,
		PriceRepo:         s.GetStores().PriceRepo,
		AddonRepo:         s.GetStores().AddonRepo,
		GroupRepo:         s.GetStores().GroupRepo,
		AnalyticsViewRepo: s.GetStores().AnalyticsViewRepo,
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

// insertUsageForCustomer inserts one meter_usage row stamped with an
// external_customer_id, for external_customer_id group-by tests.
func (s *AnalyticsServiceSuite) insertUsageForCustomer(ctx context.Context, meterID, customerID string, ts time.Time, qty float64) {
	s.NoError(s.meterUsageRepo.BulkInsertMeterUsage(ctx, []*events.MeterUsage{
		{
			Event: events.Event{
				ID:                 types.GenerateUUID(),
				TenantID:           types.GetTenantID(ctx),
				EnvironmentID:      types.GetEnvironmentID(ctx),
				ExternalCustomerID: customerID,
				Timestamp:          ts,
				EventName:          "api_call",
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

func (s *AnalyticsServiceSuite) breakdownDef() *analytics.ViewDefinition {
	return &analytics.ViewDefinition{
		Shape:      analytics.ShapeBreakdown,
		Metrics:    []string{"usage_quantity"},
		Dimensions: []string{"properties.region"},
		Filters:    []*analytics.Filter{{Field: "meter_id", Op: "eq", Value: "{{meter}}"}},
		Time:       analytics.TimeSpecRaw{Range: "{{dr}}", Grain: "day"},
		Variables: []*analytics.Variable{
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

// TestExecuteView_BreakdownByMeterIDSucceeds proves the Ruling C guard is
// gone: a structural dimension like meter_id (not just properties.*) is now
// a legal breakdown dimension, since both shapes route through the same
// detailed meter_usage engine and the shaper reads structural dims directly
// off the response item (see dimensionValue).
func (s *AnalyticsServiceSuite) TestExecuteView_BreakdownByMeterIDSucceeds() {
	ctx := s.GetContext()
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 3, nil)
	s.insertUsage(ctx, s.maxMeter.ID, s.now, 9, nil)

	def := s.breakdownDef()
	def.Dimensions = []string{"meter_id"}
	def.Filters = nil // both meters, no meter_id filter

	vars := s.drVars()
	vars["meter"] = "unused" // breakdownDef declares "meter" required even though Filters is nil here

	res, err := s.svc.ExecuteView(ctx, def, vars)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("meter_id", res.Columns[0].Name)

	totals := map[string]string{}
	for _, row := range res.Rows {
		meterID, _ := row[0].(string)
		usage, _ := row[1].(string)
		totals[meterID] = usage
	}
	s.Equal("3", totals[s.sumMeter.ID])
	s.Equal("9", totals[s.maxMeter.ID])
}

// TestExecuteView_BreakdownZeroDimensionsIsMeterLevel proves dimensions are
// optional for breakdown: an empty Dimensions list yields one row per meter
// (the engine's default), not a validation error.
func (s *AnalyticsServiceSuite) TestExecuteView_BreakdownZeroDimensionsIsMeterLevel() {
	ctx := s.GetContext()
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 3, map[string]interface{}{"region": "us"})
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 4, map[string]interface{}{"region": "eu"})

	def := s.breakdownDef()
	def.Dimensions = nil

	vars := s.drVars()
	vars["meter"] = s.sumMeter.ID

	res, err := s.svc.ExecuteView(ctx, def, vars)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Require().Len(res.Columns, 1) // usage_quantity only, no dimension columns
	s.Equal("usage_quantity", res.Columns[0].Name)
	s.Require().Len(res.Rows, 1) // one row for the meter (both events roll up)
	s.Equal("7", res.Rows[0][0])
}

// TestExecuteView_BreakdownGroupsByExternalCustomerID proves the new
// "customer_id" dimension alias (-> "external_customer_id" group_by) lets a
// breakdown view build a per-customer chart — the admin (no-customer-context)
// query path grouping raw meter_usage across all customers.
func (s *AnalyticsServiceSuite) TestExecuteView_BreakdownGroupsByExternalCustomerID() {
	ctx := s.GetContext()
	s.insertUsageForCustomer(ctx, s.sumMeter.ID, "cust_a", s.now, 3)
	s.insertUsageForCustomer(ctx, s.sumMeter.ID, "cust_b", s.now, 5)

	def := s.breakdownDef()
	def.Dimensions = []string{"customer_id"}

	vars := s.drVars()
	vars["meter"] = s.sumMeter.ID

	res, err := s.svc.ExecuteView(ctx, def, vars)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("customer_id", res.Columns[0].Name)

	totals := map[string]string{}
	for _, row := range res.Rows {
		cust, _ := row[0].(string)
		usage, _ := row[1].(string)
		totals[cust] = usage
	}
	s.Equal("3", totals["cust_a"])
	s.Equal("5", totals["cust_b"])
}

// ---------------------------------------------------------------------------
// ExecuteView — timeseries
// ---------------------------------------------------------------------------

func (s *AnalyticsServiceSuite) timeseriesDef(meterID string) *analytics.ViewDefinition {
	return &analytics.ViewDefinition{
		Shape:   analytics.ShapeTimeseries,
		Metrics: []string{"usage_quantity"},
		Filters: []*analytics.Filter{{Field: "meter_id", Op: "eq", Value: meterID}},
		Time:    analytics.TimeSpecRaw{Range: "{{dr}}", Grain: "day"},
		Variables: []*analytics.Variable{
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
	s.Require().Len(res.Rows, 1)
	s.Equal("20", res.Rows[0][1])
}

// TestExecuteView_TimeseriesWithDimensionBucketsByGroup proves timeseries
// dimensions now work as a split-by: one bucketed series per group combo,
// via the same detailed meter_usage engine as breakdown (the earlier cut
// rejected any Dimensions on a timeseries view).
func (s *AnalyticsServiceSuite) TestExecuteView_TimeseriesWithDimensionBucketsByGroup() {
	ctx := s.GetContext()
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 3, map[string]interface{}{"region": "us"})
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 5, map[string]interface{}{"region": "eu"})

	def := s.timeseriesDef(s.sumMeter.ID)
	def.Dimensions = []string{"properties.region"}

	res, err := s.svc.ExecuteView(ctx, def, s.drVars())
	s.NoError(err)
	s.Require().NotNil(res)
	s.Require().Len(res.Columns, 3)
	s.Equal("window_start", res.Columns[0].Name)
	s.Equal("properties.region", res.Columns[1].Name)
	s.Equal("usage_quantity", res.Columns[2].Name)

	totals := map[string]string{}
	for _, row := range res.Rows {
		s.Require().Len(row, 3)
		region, _ := row[1].(string)
		usage, _ := row[2].(string)
		totals[region] = usage
	}
	s.Equal("3", totals["us"])
	s.Equal("5", totals["eu"])
}

// TestExecuteView_TimeseriesMultipleMetersAllowed proves timeseries no
// longer requires exactly one meter: the admin-path engine derives each
// meter's own aggregation type independently (Ruling A still holds per
// meter — see TestExecuteView_TimeseriesUsesMeterAggregation).
func (s *AnalyticsServiceSuite) TestExecuteView_TimeseriesMultipleMetersAllowed() {
	ctx := s.GetContext()
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 3, nil)
	s.insertUsage(ctx, s.maxMeter.ID, s.now, 9, nil)

	def := s.timeseriesDef(s.sumMeter.ID)
	def.Filters = nil // no meter_id filter -> both meters

	res, err := s.svc.ExecuteView(ctx, def, s.drVars())
	s.NoError(err)
	s.Require().NotNil(res)
	s.Require().Len(res.Rows, 2) // one point per meter
}

// ---------------------------------------------------------------------------
// View CRUD
// ---------------------------------------------------------------------------

func (s *AnalyticsServiceSuite) TestCreateView_QueryView_RoundTrip() {
	ctx := s.GetContext()
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 3, map[string]interface{}{"region": "us"})
	s.insertUsage(ctx, s.sumMeter.ID, s.now, 5, map[string]interface{}{"region": "eu"})

	view := &analytics.View{
		Name:       "region breakdown",
		Definition: s.breakdownDef(),
	}
	s.NoError(s.svc.CreateView(ctx, view))
	s.NotEmpty(view.ID, "CreateView should assign an ID")

	vars := s.drVars()
	vars["meter"] = s.sumMeter.ID

	direct, err := s.svc.ExecuteView(ctx, s.breakdownDef(), vars)
	s.NoError(err)

	viaView, err := s.svc.QueryView(ctx, view.ID, vars)
	s.NoError(err)
	s.Equal(direct, viaView)
}

// TestCreateView_DefaultsVersionToOne proves the Version invariant now lives in
// the service: a caller that leaves Version unset (zero value) still gets a
// persisted view with Version == 1, regardless of what the handler does.
func (s *AnalyticsServiceSuite) TestCreateView_DefaultsVersionToOne() {
	ctx := s.GetContext()
	view := &analytics.View{
		Name:       "no version set",
		Definition: s.breakdownDef(),
	}
	s.Equal(0, view.Version, "precondition: caller left Version unset")

	s.NoError(s.svc.CreateView(ctx, view))
	s.Equal(1, view.Version, "CreateView should default Version to 1 on the in-memory value")

	stored, err := s.GetStores().AnalyticsViewRepo.Get(ctx, view.ID)
	s.NoError(err)
	s.Equal(1, stored.Version, "CreateView should persist Version == 1")
}

func (s *AnalyticsServiceSuite) TestCreateView_RejectsInvalidDefinition() {
	ctx := s.GetContext()
	view := &analytics.View{
		Name:       "invalid",
		Definition: &analytics.ViewDefinition{Shape: analytics.ShapeBreakdown}, // no metrics
	}
	err := s.svc.CreateView(ctx, view)
	s.Error(err)
	s.True(ierr.IsValidation(err))
}

func (s *AnalyticsServiceSuite) TestQueryView_CrossTenantDoesNotLeak() {
	ctx := s.GetContext()
	view := &analytics.View{
		Name:       "tenant-a view",
		Definition: s.breakdownDef(),
	}
	s.NoError(s.svc.CreateView(ctx, view))

	otherTenantCtx := types.SetTenantID(ctx, "other_tenant_xyz")

	vars := s.drVars()
	vars["meter"] = s.sumMeter.ID

	_, err := s.svc.QueryView(otherTenantCtx, view.ID, vars)
	s.Error(err)
	s.True(ierr.IsNotFound(err), "expected not-found error for cross-tenant access, got: %v", err)
}
