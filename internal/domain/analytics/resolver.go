package analytics

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// ValidateDimensions forwards to types.ValidateDimensions. Kept as a
// domain-package entry point since callers (e.g. the service layer) reach
// dimension validation through the analytics package.
func ValidateDimensions(dims []string) error {
	return types.ValidateDimensions(dims)
}

// ResolveVariables resolves a ViewDefinition against supplied variable
// values. Every variable value is a string list: a scalar is a 1-element
// list, and a date_range variable is a 1-element range-string (see
// resolveTime) that this function parses into a concrete window.
func ResolveVariables(def *types.ViewDefinition, supplied map[string][]string) (*types.ResolvedView, error) {
	if def == nil {
		return nil, ierr.NewError("view definition is required").Mark(ierr.ErrValidation)
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	for _, va := range def.Variables {
		if va == nil {
			continue
		}
		if _, ok := supplied[va.Name]; !ok && va.Required {
			return nil, ierr.NewError(fmt.Sprintf("missing required variable %q", va.Name)).Mark(ierr.ErrValidation)
		}
	}
	rv := &types.ResolvedView{Shape: def.Shape, Metrics: def.Metrics, Dimensions: def.Dimensions, Sort: def.Sort, Limit: def.Limit}

	for _, f := range def.Filters {
		if f == nil {
			continue
		}
		val, present := resolveFilterValue(f.Value, supplied)
		if !present {
			if f.Optional {
				continue // drop optional filter with no value
			}
			return nil, ierr.NewError(fmt.Sprintf("filter %q has no value", f.Field)).Mark(ierr.ErrValidation)
		}
		rv.Filters = append(rv.Filters, &types.AnalyticsFilter{Field: f.Field, Op: f.Op, Value: val})
	}

	ts, err := resolveTime(def.Time, supplied)
	if err != nil {
		return nil, err
	}
	rv.Time = ts
	return rv, nil
}

// varPlaceholder reports whether s is a "{{name}}" placeholder and, if so,
// returns name.
func varPlaceholder(s string) (string, bool) {
	if len(s) > 4 && strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}") {
		return s[2 : len(s)-2], true
	}
	return "", false
}

// resolveFilterValue resolves a filter's value list. A single-element
// "{{name}}" placeholder is replaced by the supplied variable's value
// (which may itself be multi-valued); anything else passes through as a
// literal.
func resolveFilterValue(raw []string, supplied map[string][]string) ([]string, bool) {
	if len(raw) == 1 {
		if name, ok := varPlaceholder(raw[0]); ok {
			v, present := supplied[name]
			return v, present
		}
	}
	return raw, true
}

// resolveRangeString resolves a TimeSpecRaw.Range string. A "{{name}}"
// placeholder is replaced by the first element of the supplied variable's
// value; anything else passes through as a literal.
func resolveRangeString(raw string, supplied map[string][]string) (string, bool) {
	name, isVar := varPlaceholder(raw)
	if !isVar {
		return raw, true
	}
	v, present := supplied[name]
	if !present || len(v) == 0 {
		return "", false
	}
	return v[0], true
}

// defaultRelativeRangeToken is the window used when the view's time spec is
// omitted entirely (Range is empty) — time is optional, not a hard requirement.
const defaultRelativeRangeToken = "last_7_days"

// rangeSeparator splits an absolute "<from>..<to>" range string.
const rangeSeparator = ".."

// relativeRangePattern matches "last_<N>_day(s)" / "last_<N>_hour(s)" tokens.
var relativeRangePattern = regexp.MustCompile(`^last_([0-9]+)_(day|days|hour|hours)$`)

// resolveTime resolves a view's time spec into a concrete [From, To) window.
// Range may be:
//   - "" (omitted)                     → defaults to defaultRelativeRangeToken
//   - a relative token string          → "last_N_days", "last_N_hours", "today", "yesterday"
//   - an absolute "<from>..<to>" range → "YYYY-MM-DD" dates, inclusive-from/exclusive-to
func resolveTime(raw types.TimeSpecRaw, supplied map[string][]string) (types.TimeSpec, error) {
	val, present := resolveRangeString(raw.Range, supplied)
	if !present {
		return types.TimeSpec{}, ierr.NewError("time range not supplied").Mark(ierr.ErrValidation)
	}
	if val == "" {
		val = defaultRelativeRangeToken
	}
	if strings.Contains(val, rangeSeparator) {
		parts := strings.SplitN(val, rangeSeparator, 2)
		from, err := parseDate(parts[0])
		if err != nil {
			return types.TimeSpec{}, err
		}
		to, err := parseDate(parts[1])
		if err != nil {
			return types.TimeSpec{}, err
		}
		return types.TimeSpec{From: from, To: to, Grain: raw.Grain}, nil
	}
	from, to, err := parseRelativeRange(val)
	if err != nil {
		return types.TimeSpec{}, err
	}
	return types.TimeSpec{From: from, To: to, Grain: raw.Grain}, nil
}

// parseRelativeRange resolves a relative range token into a concrete
// [from, to) UTC window anchored at time.Now(). Supported tokens:
//   - "today"          — start of today (UTC) .. now
//   - "yesterday"       — start of yesterday (UTC) .. start of today (UTC)
//   - "last_N_days"     — now-N*24h .. now
//   - "last_N_hours"    — now-N*1h .. now
func parseRelativeRange(token string) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	switch token {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, now, nil
	case "yesterday":
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return todayStart.Add(-24 * time.Hour), todayStart, nil
	}

	m := relativeRangePattern.FindStringSubmatch(token)
	if m == nil {
		return time.Time{}, time.Time{}, ierr.NewErrorf("unrecognized relative time range %q", token).
			WithHint("supported: last_N_days, last_N_hours, today, yesterday, or an absolute <from>..<to> range").
			Mark(ierr.ErrValidation)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return time.Time{}, time.Time{}, ierr.NewErrorf("invalid relative time range %q", token).Mark(ierr.ErrValidation)
	}
	var window time.Duration
	if m[2] == "day" || m[2] == "days" {
		window = time.Duration(n) * 24 * time.Hour
	} else {
		window = time.Duration(n) * time.Hour
	}
	return now.Add(-window), now, nil
}

func parseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, ierr.NewError("invalid date").WithHint("expected YYYY-MM-DD").Mark(ierr.ErrValidation)
	}
	return t.UTC(), nil
}
