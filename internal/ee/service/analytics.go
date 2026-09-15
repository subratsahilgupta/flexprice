package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/analytics"
	"github.com/flexprice/flexprice/internal/domain/events"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

type AnalyticsService interface {
	ExecuteView(ctx context.Context, def *analytics.ViewDefinition, vars map[string][]string) (*dto.AnalyticsQueryResult, error)
	CreateView(ctx context.Context, v *analytics.View) error
	QueryView(ctx context.Context, id string, vars map[string][]string) (*dto.AnalyticsQueryResult, error)
}

type analyticsService struct {
	ServiceParams
	views      analytics.Repository
	meterUsage MeterUsageService
}

func NewAnalyticsService(params ServiceParams) AnalyticsService {
	return &analyticsService{
		ServiceParams: params,
		views:         params.AnalyticsViewRepo,
		meterUsage:    NewMeterUsageService(params),
	}
}

func (s *analyticsService) ExecuteView(ctx context.Context, def *analytics.ViewDefinition, vars map[string][]string) (*dto.AnalyticsQueryResult, error) {
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
func (s *analyticsService) executeBreakdown(ctx context.Context, rv *analytics.ResolvedView) (*dto.AnalyticsQueryResult, error) {
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
func (s *analyticsService) executeTimeseries(ctx context.Context, rv *analytics.ResolvedView) (*dto.AnalyticsQueryResult, error) {
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
	if v.Definition == nil {
		return ierr.NewError("view definition is required").Mark(ierr.ErrValidation)
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

func (s *analyticsService) QueryView(ctx context.Context, id string, vars map[string][]string) (*dto.AnalyticsQueryResult, error) {
	v, err := s.views.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.ExecuteView(ctx, v.Definition, vars)
}

// translateDimension rewrites analytics-facing dimension aliases onto the
// meter_usage engine's real group_by tokens. "customer_id" is the
// analytics-facing name for the ClickHouse "external_customer_id" column.
func translateDimension(d string) string {
	if d == "customer_id" {
		return "external_customer_id"
	}
	return d
}

// translateDimensions applies translateDimension to every dim, preserving
// order and length so the shaper can still render columns using the view's
// ORIGINAL dimension names (see dimensionValue).
func translateDimensions(dims []string) []string {
	if len(dims) == 0 {
		return nil
	}
	out := make([]string, len(dims))
	for i, d := range dims {
		out[i] = translateDimension(d)
	}
	return out
}

// applyAnalyticsFilters splits a ResolvedView's filters into the typed slices
// the meter_usage params expect (meter_id/customer_id/source); anything else
// is treated as a raw property name and becomes a property filter.
func applyAnalyticsFilters(filters []*analytics.Filter, meterIDs, customerIDs, sources *[]string, props map[string][]string) {
	for _, f := range filters {
		if f == nil {
			continue
		}
		switch f.Field {
		case "meter_id":
			*meterIDs = append(*meterIDs, f.Value...)
		case "customer_id", "external_customer_id":
			*customerIDs = append(*customerIDs, f.Value...)
		case "source":
			*sources = append(*sources, f.Value...)
		default:
			props[f.Field] = append(props[f.Field], f.Value...)
		}
	}
}

// translateBreakdown maps a resolved analytics view onto the shared detailed
// meter_usage analytics params and calls MeterUsageService.GetDetailedAnalytics.
// WindowSize is deliberately left unset — breakdown is a single aggregate row
// per group, not a time series. Dimensions are optional: when empty, the
// engine already defaults to one row per meter (see
// getDetailedAnalyticsWithoutSubscriptionContext), so an empty GroupBy is not
// rejected here.
func translateBreakdown(ctx context.Context, rv *analytics.ResolvedView) (*events.MeterUsageDetailedAnalyticsParams, error) {
	if rv == nil {
		return nil, ierr.NewError("resolved view is required").Mark(ierr.ErrValidation)
	}
	if err := analytics.ValidateDimensions(rv.Dimensions); err != nil {
		return nil, err
	}
	p := &events.MeterUsageDetailedAnalyticsParams{
		TenantID:        types.GetTenantID(ctx),
		EnvironmentID:   types.GetEnvironmentID(ctx),
		StartTime:       rv.Time.From,
		EndTime:         rv.Time.To,
		GroupBy:         translateDimensions(rv.Dimensions),
		PropertyFilters: map[string][]string{},
		UseFinal:        true,
	}
	applyAnalyticsFilters(rv.Filters, &p.MeterIDs, &p.ExternalCustomerIDs, &p.Sources, p.PropertyFilters)
	return p, nil
}

// translateTimeseries maps a resolved analytics view onto the shared detailed
// meter_usage analytics params and calls MeterUsageService.GetDetailedAnalytics,
// the same engine as breakdown. WindowSize is derived from the view's
// time.grain so the response carries per-bucket Points. Dimensions act as a
// split-by (one series per group combo); multiple meters are allowed — the
// engine derives each meter's own aggregation type
// (getDetailedAnalyticsWithoutSubscriptionContext splits by AggType), so the
// caller never needs to set AggregationTypes.
func translateTimeseries(ctx context.Context, rv *analytics.ResolvedView) (*events.MeterUsageDetailedAnalyticsParams, error) {
	if rv == nil {
		return nil, ierr.NewError("resolved view is required").Mark(ierr.ErrValidation)
	}
	if err := analytics.ValidateDimensions(rv.Dimensions); err != nil {
		return nil, err
	}
	p := &events.MeterUsageDetailedAnalyticsParams{
		TenantID:        types.GetTenantID(ctx),
		EnvironmentID:   types.GetEnvironmentID(ctx),
		StartTime:       rv.Time.From,
		EndTime:         rv.Time.To,
		WindowSize:      rv.Time.Grain.ToWindowSize(),
		GroupBy:         translateDimensions(rv.Dimensions),
		PropertyFilters: map[string][]string{},
		UseFinal:        true,
	}
	applyAnalyticsFilters(rv.Filters, &p.MeterIDs, &p.ExternalCustomerIDs, &p.Sources, p.PropertyFilters)
	return p, nil
}
