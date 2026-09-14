package analytics

import (
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
)

type Shape string

const (
	ShapeTimeseries Shape = "timeseries"
	ShapeBreakdown  Shape = "breakdown"
)

type Filter struct {
	Field    string `json:"field"`
	Op       string `json:"op"`
	Value    any    `json:"value"`
	Optional bool   `json:"optional,omitempty"`
}

type SortSpec struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}

type Variable struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Default  any    `json:"default,omitempty"`
}

// TimeSpecRaw is the unresolved time spec. Range may be a "{{var}}"
// placeholder, an absolute {"from":"YYYY-MM-DD","to":"YYYY-MM-DD"} map, a
// relative range token (see resolveTime — "last_N_days", "last_N_hours",
// "today", "yesterday"), or omitted entirely (defaults to "last_7_days").
type TimeSpecRaw struct {
	Range any    `json:"range,omitempty"`
	Grain string `json:"grain"`
}

type TimeSpec struct {
	From  time.Time
	To    time.Time
	Grain string
}

type ViewDefinition struct {
	Name       string      `json:"name"`
	Shape      Shape       `json:"shape"`
	Metrics    []string    `json:"metrics"`
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
	metrics []string,
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
	return nil
}
