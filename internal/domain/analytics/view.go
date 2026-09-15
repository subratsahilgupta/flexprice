package analytics

import (
	"regexp"
	"strings"
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

type Shape string

const (
	ShapeTimeseries Shape = "timeseries"
	ShapeBreakdown  Shape = "breakdown"
)

// Grain is the timeseries bucketing granularity. It is the wire-facing enum;
// ToWindowSize maps it onto the engine's internal types.WindowSize.
type Grain string

const (
	GrainHour  Grain = "hour"
	GrainDay   Grain = "day"
	GrainWeek  Grain = "week"
	GrainMonth Grain = "month"
)

func (g Grain) Validate() error {
	switch g {
	case "", GrainHour, GrainDay, GrainWeek, GrainMonth:
		return nil
	default:
		return ierr.NewErrorf("invalid grain %q", g).
			WithHint("grain must be one of: hour, day, week, month").
			Mark(ierr.ErrValidation)
	}
}

// ToWindowSize maps a validated Grain onto the meter_usage engine's
// types.WindowSize. An empty (unset) grain maps to an empty window size.
func (g Grain) ToWindowSize() types.WindowSize {
	switch g {
	case GrainHour:
		return types.WindowSizeHour
	case GrainDay:
		return types.WindowSizeDay
	case GrainWeek:
		return types.WindowSizeWeek
	case GrainMonth:
		return types.WindowSizeMonth
	default:
		return ""
	}
}

// VariableType is the declared type of a view Variable placeholder.
type VariableType string

const (
	VariableTypeDateRange  VariableType = "date_range"
	VariableTypeString     VariableType = "string"
	VariableTypeStringList VariableType = "string_list"
	VariableTypeNumber     VariableType = "number"
	VariableTypeEnum       VariableType = "enum"
	VariableTypeBoolean    VariableType = "boolean"
)

func (t VariableType) Validate() error {
	switch t {
	case VariableTypeDateRange, VariableTypeString, VariableTypeStringList, VariableTypeNumber, VariableTypeEnum, VariableTypeBoolean:
		return nil
	default:
		return ierr.NewErrorf("invalid variable type %q", t).
			WithHint("type must be one of: date_range, string, string_list, number, enum, boolean").
			Mark(ierr.ErrValidation)
	}
}

// Metric is a Phase-1 usage-only metric a view can request.
type Metric string

const (
	MetricUsageQuantity Metric = "usage_quantity"
	MetricEventCount    Metric = "event_count"
)

func (m Metric) Validate() error {
	switch m {
	case MetricUsageQuantity, MetricEventCount:
		return nil
	default:
		return ierr.NewErrorf("invalid metric %q", m).
			WithHint("metric must be one of: usage_quantity, event_count").
			Mark(ierr.ErrValidation)
	}
}

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

type Filter struct {
	Field    string                   `json:"field"`
	Op       types.FilterOperatorType `json:"op"`
	Value    []string                 `json:"value"`
	Optional bool                     `json:"optional,omitempty"`
}

type SortSpec struct {
	Field string              `json:"field"`
	Dir   types.SortDirection `json:"dir"`
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
	Name       string      `json:"name"`
	Shape      Shape       `json:"shape"`
	Metrics    []Metric    `json:"metrics"`
	Dimensions []string    `json:"dimensions,omitempty"`
	Filters    []*Filter   `json:"filters,omitempty"`
	Time       TimeSpecRaw `json:"time"`
	Sort       []*SortSpec `json:"sort,omitempty"`
	Limit      int         `json:"limit,omitempty"`
	Variables  []*Variable `json:"variables,omitempty"`
}

// NewViewDefinition builds a ViewDefinition from its constituent parts.
func NewViewDefinition(
	name string,
	shape Shape,
	metrics []Metric,
	dimensions []string,
	filters []*Filter,
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
		case types.SortDirectionAsc, types.SortDirectionDesc:
		default:
			return ierr.NewErrorf("invalid sort direction %q", s.Dir).
				WithHint("dir must be one of: asc, desc").
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
