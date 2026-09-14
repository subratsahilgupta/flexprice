package analytics

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
)

type ResolvedView struct {
	Shape      Shape
	Metrics    []string
	Dimensions []string
	Filters    []*Filter // concrete values; optional-with-unsupplied dropped
	Time       TimeSpec
	Sort       []*SortSpec
	Limit      int
}

func ResolveVariables(def *ViewDefinition, supplied map[string]any) (*ResolvedView, error) {
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
	rv := &ResolvedView{Shape: def.Shape, Metrics: def.Metrics, Dimensions: def.Dimensions, Sort: def.Sort, Limit: def.Limit}

	for _, f := range def.Filters {
		if f == nil {
			continue
		}
		val, present := resolveValue(f.Value, supplied)
		if !present {
			if f.Optional {
				continue // drop optional filter with no value
			}
			return nil, ierr.NewError(fmt.Sprintf("filter %q has no value", f.Field)).Mark(ierr.ErrValidation)
		}
		rv.Filters = append(rv.Filters, &Filter{Field: f.Field, Op: f.Op, Value: val})
	}

	ts, err := resolveTime(def.Time, supplied)
	if err != nil {
		return nil, err
	}
	rv.Time = ts
	return rv, nil
}

// resolveValue returns (value, present). A "{{name}}" looks the var up in supplied.
func resolveValue(raw any, supplied map[string]any) (any, bool) {
	s, ok := raw.(string)
	if ok && len(s) > 4 && s[:2] == "{{" && s[len(s)-2:] == "}}" {
		name := s[2 : len(s)-2]
		v, present := supplied[name]
		return v, present
	}
	return raw, true // literal
}

// defaultRelativeRangeToken is the window used when the view's time spec is
// omitted entirely (Range is nil) — time is optional, not a hard requirement.
const defaultRelativeRangeToken = "last_7_days"

// relativeRangePattern matches "last_<N>_day(s)" / "last_<N>_hour(s)" tokens.
var relativeRangePattern = regexp.MustCompile(`^last_([0-9]+)_(day|days|hour|hours)$`)

// resolveTime resolves a view's time spec into a concrete [From, To) window.
// Range may be:
//   - nil (omitted)                → defaults to defaultRelativeRangeToken
//   - a relative token string      → "last_N_days", "last_N_hours", "today", "yesterday"
//   - an absolute {from,to} map    → "YYYY-MM-DD" dates, inclusive-from/exclusive-to
func resolveTime(raw TimeSpecRaw, supplied map[string]any) (TimeSpec, error) {
	val, present := resolveValue(raw.Range, supplied)
	if !present {
		return TimeSpec{}, ierr.NewError("time range not supplied").Mark(ierr.ErrValidation)
	}
	if val == nil {
		from, to, err := parseRelativeRange(defaultRelativeRangeToken)
		if err != nil {
			return TimeSpec{}, err
		}
		return TimeSpec{From: from, To: to, Grain: raw.Grain}, nil
	}
	if token, ok := val.(string); ok {
		from, to, err := parseRelativeRange(token)
		if err != nil {
			return TimeSpec{}, err
		}
		return TimeSpec{From: from, To: to, Grain: raw.Grain}, nil
	}
	m, ok := val.(map[string]any)
	if !ok {
		return TimeSpec{}, ierr.NewError("time range must be {from,to} or a relative range token").
			WithHint("supported: last_N_days, last_N_hours, today, yesterday, or an absolute {from,to} range").
			Mark(ierr.ErrValidation)
	}
	from, err := parseDate(m["from"])
	if err != nil {
		return TimeSpec{}, err
	}
	to, err := parseDate(m["to"])
	if err != nil {
		return TimeSpec{}, err
	}
	return TimeSpec{From: from, To: to, Grain: raw.Grain}, nil
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
			WithHint("supported: last_N_days, last_N_hours, today, yesterday, or an absolute {from,to} range").
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

func parseDate(v any) (time.Time, error) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, ierr.NewError("date must be a string").Mark(ierr.ErrValidation)
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, ierr.NewError("invalid date").WithHint("expected YYYY-MM-DD").Mark(ierr.ErrValidation)
	}
	return t.UTC(), nil
}
