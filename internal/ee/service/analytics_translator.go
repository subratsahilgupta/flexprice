package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/flexprice/flexprice/internal/domain/analytics"
	"github.com/flexprice/flexprice/internal/domain/events"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// validAnalyticsDimension mirrors the meter_usage query builder's group_by
// allowlist: meter_id, source, properties.<field>.
var validAnalyticsDimension = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

func validateAnalyticsDimensions(dims []string) error {
	for _, d := range dims {
		if !validAnalyticsDimension.MatchString(d) {
			return ierr.NewErrorf("illegal dimension: %q", d).
				WithHint("dimensions must match [A-Za-z0-9_.]+").
				Mark(ierr.ErrValidation)
		}
	}
	return nil
}

// applyAnalyticsFilters splits a ResolvedView's filters into the typed slices
// the meter_usage params expect (meter_id/customer_id/source); anything else
// is treated as a raw property name and becomes a property filter.
func applyAnalyticsFilters(filters []analytics.Filter, meterIDs, customerIDs, sources *[]string, props map[string][]string) {
	for _, f := range filters {
		vals := toAnalyticsStringSlice(f.Value)
		switch f.Field {
		case "meter_id":
			*meterIDs = append(*meterIDs, vals...)
		case "customer_id":
			*customerIDs = append(*customerIDs, vals...)
		case "source":
			*sources = append(*sources, vals...)
		default:
			props[f.Field] = append(props[f.Field], vals...)
		}
	}
}

// grainToWindowSize maps a TimeSpec.Grain ("day", "hour", "15min", ...) onto
// types.WindowSize, whose constants are the same names upper-cased. Empty
// grain maps to an empty (unset) window size.
func grainToWindowSize(grain string) (types.WindowSize, error) {
	w := types.WindowSize(strings.ToUpper(grain))
	if err := w.Validate(); err != nil {
		return "", err
	}
	return w, nil
}

func toAnalyticsStringSlice(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// TranslateBreakdown maps a resolved analytics view onto the existing
// detailed meter_usage analytics params, injecting tenant/environment RLS
// from ctx. It invents no new query logic: the existing engine (and its
// meter-aggregation defaulting for AggregationTypes) does the rest.
func TranslateBreakdown(ctx context.Context, rv analytics.ResolvedView) (*events.MeterUsageDetailedAnalyticsParams, error) {
	if err := validateAnalyticsDimensions(rv.Dimensions); err != nil {
		return nil, err
	}
	windowSize, err := grainToWindowSize(rv.Time.Grain)
	if err != nil {
		return nil, err
	}
	p := &events.MeterUsageDetailedAnalyticsParams{
		TenantID:        types.GetTenantID(ctx),
		EnvironmentID:   types.GetEnvironmentID(ctx),
		StartTime:       rv.Time.From,
		EndTime:         rv.Time.To,
		WindowSize:      windowSize,
		GroupBy:         rv.Dimensions,
		PropertyFilters: map[string][]string{},
		UseFinal:        true,
	}
	applyAnalyticsFilters(rv.Filters, &p.MeterIDs, &p.ExternalCustomerIDs, &p.Sources, p.PropertyFilters)
	return p, nil
}

// TranslateTimeseries maps a resolved analytics view onto the existing
// meter_usage query params, injecting tenant/environment RLS from ctx.
func TranslateTimeseries(ctx context.Context, rv analytics.ResolvedView) (*events.MeterUsageQueryParams, error) {
	if err := validateAnalyticsDimensions(rv.Dimensions); err != nil {
		return nil, err
	}
	windowSize, err := grainToWindowSize(rv.Time.Grain)
	if err != nil {
		return nil, err
	}
	p := &events.MeterUsageQueryParams{
		TenantID:        types.GetTenantID(ctx),
		EnvironmentID:   types.GetEnvironmentID(ctx),
		StartTime:       rv.Time.From,
		EndTime:         rv.Time.To,
		WindowSize:      windowSize,
		GroupBy:         rv.Dimensions,
		PropertyFilters: map[string][]string{},
		UseFinal:        true,
	}
	applyAnalyticsFilters(rv.Filters, &p.MeterIDs, &p.ExternalCustomerIDs, &p.Sources, p.PropertyFilters)
	return p, nil
}
