package types

import (
	"regexp"
	"strings"
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
)

// validPropertyField matches the charset allowed after "properties.".
var validPropertyField = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

// ValidateDimensions checks each dimension against the meter_usage engine's
// group_by allowlist: meter_id, source, external_customer_id, customer_id
// (analytics-facing alias), or properties.<field>.
func ValidateDimensions(dims []string) error {
	for _, d := range dims {
		if err := validateDimension(d); err != nil {
			return err
		}
	}
	return nil
}

func validateDimension(d string) error {
	switch d {
	case "meter_id", "source", "external_customer_id", "customer_id":
		return nil
	}
	if field, ok := strings.CutPrefix(d, "properties."); ok && field != "" && validPropertyField.MatchString(field) {
		return nil
	}
	return ierr.NewErrorf("illegal dimension: %q", d).
		WithHint("dimensions must be one of meter_id, source, external_customer_id, customer_id, or properties.<field>").
		Mark(ierr.ErrValidation)
}

// AnalyticsFilter is a single filter clause of an analytics ViewDefinition.
// Named "Analytics"-prefixed to avoid colliding with the existing
// (deprecated) pagination Filter in this package.
type AnalyticsFilter struct {
	Field    string             `json:"field"`
	Op       FilterOperatorType `json:"op"`
	Value    []string           `json:"value"`
	Optional bool               `json:"optional,omitempty"`
}

type SortSpec struct {
	Field string        `json:"field"`
	Dir   SortDirection `json:"dir"`
}

type Variable struct {
	Name     string       `json:"name"`
	Type     VariableType `json:"type"`
	Required bool         `json:"required,omitempty"`
	Default  []string     `json:"default,omitempty"`
}

// TimeSpecRaw is the unresolved time spec. Range is a single string: a
// "{{var}}" placeholder, a relative range token (see resolveTime —
// "last_N_days", "last_N_hours", "today", "yesterday"), an absolute
// "<from>..<to>" range ("YYYY-MM-DD" dates), or omitted entirely (defaults
// to "last_7_days").
type TimeSpecRaw struct {
	Range string `json:"range,omitempty"`
	Grain Grain  `json:"grain"`
}

type TimeSpec struct {
	From  time.Time
	To    time.Time
	Grain Grain
}

type ViewDefinition struct {
	Name       string             `json:"name"`
	Shape      Shape              `json:"shape"`
	Metrics    []Metric           `json:"metrics"`
	Dimensions []string           `json:"dimensions,omitempty"`
	Filters    []*AnalyticsFilter `json:"filters,omitempty"`
	Time       TimeSpecRaw        `json:"time"`
	Sort       []*SortSpec        `json:"sort,omitempty"`
	Limit      int                `json:"limit,omitempty"`
	Variables  []*Variable        `json:"variables,omitempty"`
}

// NewViewDefinition builds a ViewDefinition from its constituent parts.
func NewViewDefinition(
	name string,
	shape Shape,
	metrics []Metric,
	dimensions []string,
	filters []*AnalyticsFilter,
	timeSpec TimeSpecRaw,
	sort []*SortSpec,
	limit int,
	variables []*Variable,
) ViewDefinition {
	return ViewDefinition{
		Name:       name,
		Shape:      shape,
		Metrics:    metrics,
		Dimensions: dimensions,
		Filters:    filters,
		Time:       timeSpec,
		Sort:       sort,
		Limit:      limit,
		Variables:  variables,
	}
}

func (v ViewDefinition) Validate() error {
	switch v.Shape {
	case ShapeTimeseries, ShapeBreakdown:
	default:
		return ierr.NewError("unsupported shape").
			WithHint("shape must be one of: timeseries, breakdown").
			Mark(ierr.ErrValidation)
	}
	if len(v.Metrics) == 0 {
		return ierr.NewError("at least one metric is required").Mark(ierr.ErrValidation)
	}
	for _, m := range v.Metrics {
		if err := m.Validate(); err != nil {
			return err
		}
	}
	if err := ValidateDimensions(v.Dimensions); err != nil {
		return err
	}
	if err := v.Time.Grain.Validate(); err != nil {
		return err
	}
	for _, s := range v.Sort {
		if s == nil {
			continue
		}
		switch s.Dir {
		case "", SortDirectionAsc, SortDirectionDesc:
		default:
			return ierr.NewErrorf("invalid sort direction %q", s.Dir).
				WithHint("dir must be one of: asc, desc (empty defaults to asc)").
				Mark(ierr.ErrValidation)
		}
	}
	for _, va := range v.Variables {
		if va == nil {
			continue
		}
		if err := va.Type.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ResolvedView is a ViewDefinition with all variables resolved to concrete
// values — built by domain/analytics.ResolveVariables.
type ResolvedView struct {
	Shape      Shape
	Metrics    []Metric
	Dimensions []string
	Filters    []*AnalyticsFilter // concrete values; optional-with-unsupplied dropped
	Time       TimeSpec
	Sort       []*SortSpec
	Limit      int
}
