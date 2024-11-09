package meter

import (
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/models"
)

type AggregationType string

// Note: keep values up to date in the meter package
const (
	AggregationTypeCount       AggregationType = "COUNT"
	AggregationTypeSum         AggregationType = "SUM"
	AggregationTypeAvg         AggregationType = "AVG"
	AggregationTypeMax         AggregationType = "MAX"
	AggregationTypeMin         AggregationType = "MIN"
	AggregationTypeCountUnique AggregationType = "COUNT_UNIQUE"
)

type WindowSize string

// Note: keep values up to date in the meter package
const (
	WindowSizeMinute WindowSize = "MINUTE"
	WindowSizeHour   WindowSize = "HOUR"
	WindowSizeDay    WindowSize = "DAY"
)

// Duration returns the duration of the window size
func (w WindowSize) Duration() time.Duration {
	var windowDuration time.Duration
	switch w {
	case WindowSizeMinute:
		windowDuration = time.Minute
	case WindowSizeHour:
		windowDuration = time.Hour
	case WindowSizeDay:
		windowDuration = 24 * time.Hour
	}

	return windowDuration
}

func WindowSizeFromDuration(duration time.Duration) (WindowSize, error) {
	switch duration.Minutes() {
	case time.Minute.Minutes():
		return WindowSizeMinute, nil
	case time.Hour.Minutes():
		return WindowSizeHour, nil
	case 24 * time.Hour.Minutes():
		return WindowSizeDay, nil
	default:
		return "", fmt.Errorf("invalid window size duration: %s", duration)
	}
}

type Meter struct {
	ID               string           `db:"id" json:"id"`
	Filters          []MeterFilter    `db:"filters" json:"filters"`
	Aggregation      MeterAggregation `db:"aggregation" json:"aggregation"`
	WindowSize       WindowSize       `json:"windowSize,omitempty" yaml:"windowSize,omitempty"`
	models.BaseModel                  // Embed the base model
}

type MeterFilter struct {
	Conditions []MeterCondition `json:"conditions"`
}

type MeterCondition struct {
	Field     string `json:"field"`
	Operation string `json:"operation"`
	Value     string `json:"value"`
}

type MeterAggregation struct {
	Function string `json:"function"`
	Field    string `json:"field"`
}

// Constructor for creating new meters with defaults
func NewMeter(id string, createdBy string) *Meter {
	now := time.Now()
	return &Meter{
		ID: id,
		BaseModel: models.BaseModel{
			CreatedAt: now,
			UpdatedAt: now,
			CreatedBy: createdBy,
			UpdatedBy: createdBy,
			Status:    models.StatusActive,
		},
	}
}
