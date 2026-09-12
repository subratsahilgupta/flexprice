package service

import (
	"context"
	"strings"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/analytics"
	"github.com/flexprice/flexprice/internal/domain/events"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// AnalyticsService is the integration hub for Phase-1 analytics: it resolves a
// view definition against variables, translates + executes it through the
// existing meter_usage query engine, and shapes the result for rendering. It
// also owns saved-view CRUD.
type AnalyticsService interface {
	ExecuteView(ctx context.Context, def analytics.ViewDefinition, vars map[string]any) (*QueryResult, error)
	CreateView(ctx context.Context, v *analytics.SavedView) error
	QuerySavedView(ctx context.Context, id string, vars map[string]any) (*QueryResult, error)
}

type analyticsService struct {
	ServiceParams
	savedViews analytics.Repository
	meterUsage MeterUsageService
}

// NewAnalyticsService constructs the AnalyticsService. The saved-view repo is
// threaded through ServiceParams.AnalyticsSavedViewRepo.
func NewAnalyticsService(params ServiceParams) AnalyticsService {
	return &analyticsService{
		ServiceParams: params,
		savedViews:    params.AnalyticsSavedViewRepo,
		meterUsage:    NewMeterUsageService(params),
	}
}

func (s *analyticsService) ExecuteView(ctx context.Context, def analytics.ViewDefinition, vars map[string]any) (*QueryResult, error) {
	rv, err := analytics.ResolveVariables(def, vars)
	if err != nil {
		return nil, err
	}

	switch rv.Shape {
	case analytics.ShapeBreakdown:
		return s.executeBreakdown(ctx, rv)
	case analytics.ShapeTimeseries:
		return s.executeTimeseries(ctx, rv)
	default:
		return nil, ierr.NewErrorf("unsupported shape %q", rv.Shape).Mark(ierr.ErrValidation)
	}
}

// executeBreakdown translates + executes a breakdown view through
// MeterUsageService.GetDetailedAnalytics, which (for admin-style queries with
// no external_customer_id) resolves each meter's own aggregation type via
// getDetailedAnalyticsWithoutSubscriptionContext — so, unlike the timeseries
// path, no manual aggregation-type derivation is needed here.
func (s *analyticsService) executeBreakdown(ctx context.Context, rv analytics.ResolvedView) (*QueryResult, error) {
	if err := requirePropertyDimensions(rv.Dimensions); err != nil {
		return nil, err
	}

	params, err := TranslateBreakdown(ctx, rv)
	if err != nil {
		return nil, err
	}

	resp, err := s.meterUsage.GetDetailedAnalytics(ctx, params)
	if err != nil {
		s.Logger.Error(ctx, "analytics breakdown execution failed", "error", err)
		return nil, err
	}

	res := ShapeBreakdown(toDetailedResults(resp.Items), rv.Dimensions)
	return &res, nil
}

// executeTimeseries requires the view to target exactly one meter so the
// meter's real aggregation type can be resolved and set on the query params.
// It calls MeterUsageRepo.GetUsage directly rather than
// MeterUsageService.GetUsage: the service method gates results on the meter
// being an active subscription line item for a supplied external_customer_id
// (activeSubscriptionMeterIDs), and Phase-1 admin-style views have no
// customer filter — going through the service would always return a zeroed
// result. Calling the repo directly mirrors the breakdown path's admin-style
// (no subscription context) execution.
func (s *analyticsService) executeTimeseries(ctx context.Context, rv analytics.ResolvedView) (*QueryResult, error) {
	meterID, err := singleMeterID(rv.Filters)
	if err != nil {
		return nil, err
	}

	m, err := s.MeterRepo.GetMeter(ctx, meterID)
	if err != nil {
		return nil, err
	}

	params, err := TranslateTimeseries(ctx, rv)
	if err != nil {
		return nil, err
	}
	params.MeterID = meterID
	params.AggregationType = m.Aggregation.Type

	agg, err := s.MeterUsageRepo.GetUsage(ctx, params)
	if err != nil {
		s.Logger.Error(ctx, "analytics timeseries execution failed", "error", err, "meter_id", meterID)
		return nil, err
	}

	res := ShapeTimeseries(agg)
	return &res, nil
}

func (s *analyticsService) CreateView(ctx context.Context, v *analytics.SavedView) error {
	if v == nil {
		return ierr.NewError("saved view is required").Mark(ierr.ErrValidation)
	}
	if err := v.Definition.Validate(); err != nil {
		return err
	}
	if v.ID == "" {
		v.ID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ANALYTICS_SAVED_VIEW)
	}
	if v.Version == 0 {
		v.Version = 1
	}
	if err := s.savedViews.Create(ctx, v); err != nil {
		s.Logger.Error(ctx, "failed to create analytics saved view", "error", err, "saved_view_id", v.ID)
		return err
	}
	return nil
}

func (s *analyticsService) QuerySavedView(ctx context.Context, id string, vars map[string]any) (*QueryResult, error) {
	v, err := s.savedViews.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.ExecuteView(ctx, v.Definition, vars)
}

// requirePropertyDimensions enforces Ruling C: Phase-1 breakdown dimensions
// are property-based only. A dimension that ShapeBreakdown can't map (e.g. a
// structural-entity dimension like feature_id, which needs a join — out of
// scope here) must fail loudly rather than silently shape to empty strings.
func requirePropertyDimensions(dims []string) error {
	for _, d := range dims {
		if !strings.HasPrefix(d, "properties.") {
			return ierr.NewErrorf("unsupported breakdown dimension %q", d).
				WithHint("phase-1 breakdown views only support properties.* dimensions").
				Mark(ierr.ErrValidation)
		}
	}
	return nil
}

// singleMeterID extracts the one meter_id a timeseries view must filter on
// (Ruling A: phase-1 timeseries requires exactly one meter so its real
// aggregation type can be resolved).
func singleMeterID(filters []analytics.Filter) (string, error) {
	var ids []string
	for _, f := range filters {
		if f.Field == "meter_id" {
			ids = append(ids, toAnalyticsStringSlice(f.Value)...)
		}
	}
	ids = lo.Uniq(ids)
	if len(ids) != 1 {
		return "", ierr.NewError("timeseries requires exactly one meter").
			WithHint("filter on exactly one meter_id for timeseries views").
			Mark(ierr.ErrValidation)
	}
	return ids[0], nil
}

// toDetailedResults unwraps the dto response from MeterUsageService.GetDetailedAnalytics
// into the []events.MeterUsageDetailedResult shape ShapeBreakdown consumes.
func toDetailedResults(items []dto.UsageAnalyticItem) []events.MeterUsageDetailedResult {
	out := make([]events.MeterUsageDetailedResult, 0, len(items))
	for _, it := range items {
		out = append(out, events.MeterUsageDetailedResult{
			MeterID:    it.MeterID,
			Source:     it.Source,
			Sources:    it.Sources,
			Properties: it.Properties,
			TotalUsage: it.TotalUsage,
			EventCount: it.EventCount,
		})
	}
	return out
}
