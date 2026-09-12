package analytics

import (
	"fmt"
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
)

type ResolvedView struct {
	Shape      Shape
	Metrics    []string
	Dimensions []string
	Filters    []Filter // concrete values; optional-with-unsupplied dropped
	Time       TimeSpec
	Sort       []SortSpec
	Limit      int
}

func ResolveVariables(def ViewDefinition, supplied map[string]any) (ResolvedView, error) {
	if err := def.Validate(); err != nil {
		return ResolvedView{}, err
	}
	for _, va := range def.Variables {
		if _, ok := supplied[va.Name]; !ok && va.Required {
			return ResolvedView{}, ierr.NewError(fmt.Sprintf("missing required variable %q", va.Name)).Mark(ierr.ErrValidation)
		}
	}
	rv := ResolvedView{Shape: def.Shape, Metrics: def.Metrics, Dimensions: def.Dimensions, Sort: def.Sort, Limit: def.Limit}

	for _, f := range def.Filters {
		val, present := resolveValue(f.Value, supplied)
		if !present {
			if f.Optional {
				continue // drop optional filter with no value
			}
			return ResolvedView{}, ierr.NewError(fmt.Sprintf("filter %q has no value", f.Field)).Mark(ierr.ErrValidation)
		}
		rv.Filters = append(rv.Filters, Filter{Field: f.Field, Op: f.Op, Value: val})
	}

	ts, err := resolveTime(def.Time, supplied)
	if err != nil {
		return ResolvedView{}, err
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

func resolveTime(raw TimeSpecRaw, supplied map[string]any) (TimeSpec, error) {
	val, present := resolveValue(raw.Range, supplied)
	if !present {
		return TimeSpec{}, ierr.NewError("time range not supplied").Mark(ierr.ErrValidation)
	}
	m, ok := val.(map[string]any)
	if !ok {
		return TimeSpec{}, ierr.NewError("time range must be {from,to}").Mark(ierr.ErrValidation)
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
