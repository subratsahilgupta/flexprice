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
// allowlist: meter_id, source, external_customer_id, properties.<field>.
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
func applyAnalyticsFilters(filters []analytics.Filter, meterIDs, customerIDs, sources *[]string, props map[string][]string) {
	for _, f := range filters {
		vals := toAnalyticsStringSlice(f.Value)
		switch f.Field {
		case "meter_id":
			*meterIDs = append(*meterIDs, vals...)
		case "customer_id", "external_customer_id":
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
	if grain == "" {
		return "", nil
	}
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

// translateBreakdown maps a resolved analytics view onto the shared detailed
// meter_usage analytics params and calls MeterUsageService.GetDetailedAnalytics.
// WindowSize is deliberately left unset — breakdown is a single aggregate row
// per group, not a time series. Dimensions are optional: when empty, the
// engine already defaults to one row per meter (see
// getDetailedAnalyticsWithoutSubscriptionContext), so an empty GroupBy is not
// rejected here.
func translateBreakdown(ctx context.Context, rv analytics.ResolvedView) (*events.MeterUsageDetailedAnalyticsParams, error) {
	if err := validateAnalyticsDimensions(rv.Dimensions); err != nil {
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
func translateTimeseries(ctx context.Context, rv analytics.ResolvedView) (*events.MeterUsageDetailedAnalyticsParams, error) {
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
		GroupBy:         translateDimensions(rv.Dimensions),
		PropertyFilters: map[string][]string{},
		UseFinal:        true,
	}
	applyAnalyticsFilters(rv.Filters, &p.MeterIDs, &p.ExternalCustomerIDs, &p.Sources, p.PropertyFilters)
	return p, nil
}
