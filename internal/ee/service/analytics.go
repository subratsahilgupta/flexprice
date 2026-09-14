package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/analytics"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// AnalyticsService is the integration hub for Phase-1 analytics: it resolves a
// view definition against variables, translates + executes it through the
// existing meter_usage query engine, and shapes the result for rendering. It
// also owns view CRUD.
type AnalyticsService interface {
	ExecuteView(ctx context.Context, def analytics.ViewDefinition, vars map[string]any) (*dto.AnalyticsQueryResult, error)
	CreateView(ctx context.Context, v *analytics.View) error
	QueryView(ctx context.Context, id string, vars map[string]any) (*dto.AnalyticsQueryResult, error)
}

type analyticsService struct {
	ServiceParams
	views      analytics.Repository
	meterUsage MeterUsageService
}

// NewAnalyticsService constructs the AnalyticsService. The view repo is
// threaded through ServiceParams.AnalyticsViewRepo.
func NewAnalyticsService(params ServiceParams) AnalyticsService {
	return &analyticsService{
		ServiceParams: params,
		views:         params.AnalyticsViewRepo,
		meterUsage:    NewMeterUsageService(params),
	}
}

func (s *analyticsService) ExecuteView(ctx context.Context, def analytics.ViewDefinition, vars map[string]any) (*dto.AnalyticsQueryResult, error) {
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
// getDetailedAnalyticsWithoutSubscriptionContext. Dimensions are optional —
// an empty GroupBy yields one row per meter.
func (s *analyticsService) executeBreakdown(ctx context.Context, rv analytics.ResolvedView) (*dto.AnalyticsQueryResult, error) {
	params, err := translateBreakdown(ctx, rv)
	if err != nil {
		return nil, err
	}

	resp, err := s.meterUsage.GetDetailedAnalytics(ctx, params)
	if err != nil {
		s.Logger.Error(ctx, "analytics breakdown execution failed", "error", err)
		return nil, err
	}

	res := shapeBreakdown(resp.Items, rv.Dimensions)
	return &res, nil
}

// executeTimeseries translates + executes a timeseries view through the same
// MeterUsageService.GetDetailedAnalytics engine as breakdown (unlike the
// earlier Phase-1 cut, which called MeterUsageRepo.GetUsage directly and
// required exactly one meter with no dimensions). Dimensions act as a
// split-by; multiple meters are allowed since the engine derives each
// meter's real aggregation type on its own.
func (s *analyticsService) executeTimeseries(ctx context.Context, rv analytics.ResolvedView) (*dto.AnalyticsQueryResult, error) {
	params, err := translateTimeseries(ctx, rv)
	if err != nil {
		return nil, err
	}

	resp, err := s.meterUsage.GetDetailedAnalytics(ctx, params)
	if err != nil {
		s.Logger.Error(ctx, "analytics timeseries execution failed", "error", err)
		return nil, err
	}

	res := shapeTimeseries(resp.Items, rv.Dimensions)
	return &res, nil
}

func (s *analyticsService) CreateView(ctx context.Context, v *analytics.View) error {
	if v == nil {
		return ierr.NewError("view is required").Mark(ierr.ErrValidation)
	}
	if err := v.Definition.Validate(); err != nil {
		return err
	}
	if v.ID == "" {
		v.ID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ANALYTICS_VIEW)
	}
	if v.Version == 0 {
		v.Version = 1
	}
	if err := s.views.Create(ctx, v); err != nil {
		s.Logger.Error(ctx, "failed to create analytics view", "error", err, "view_id", v.ID)
		return err
	}
	return nil
}

func (s *analyticsService) QueryView(ctx context.Context, id string, vars map[string]any) (*dto.AnalyticsQueryResult, error) {
	v, err := s.views.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.ExecuteView(ctx, v.Definition, vars)
}
