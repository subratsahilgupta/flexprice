package meter

import (
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/google/uuid"
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
	ID          string      `db:"id" json:"id"`
	TenantID    string      `db:"tenant_id" json:"tenant_id,omitempty"`
	EventName   string      `db:"event_name" json:"event_name"`
	Aggregation Aggregation `db:"aggregation" json:"aggregation"`
	WindowSize  WindowSize  `db:"window_size" json:"window_size"`
	types.BaseModel
}

type Aggregation struct {
	Type  types.AggregationType `json:"type"`
	Field string                `json:"field,omitempty"`
}

// Validate validates the meter configuration
func (m *Meter) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("id is required")
	}
	if m.EventName == "" {
		return fmt.Errorf("event_name is required")
	}
	if !m.Aggregation.Type.Validate() {
		return fmt.Errorf("invalid aggregation type: %s", m.Aggregation.Type)
	}
	if m.Aggregation.Type.RequiresField() && m.Aggregation.Field == "" {
		return fmt.Errorf("field is required for aggregation type: %s", m.Aggregation.Type)
	}
	return nil
}

// Constructor for creating new meters with defaults
func NewMeter(id string, createdBy string) *Meter {
	now := time.Now().UTC()
	if id == "" {
		id = uuid.New().String()
	}

	return &Meter{
		ID: id,
		BaseModel: types.BaseModel{
			CreatedAt: now,
			UpdatedAt: now,
			CreatedBy: createdBy,
			UpdatedBy: createdBy,
			Status:    types.StatusActive,
		},
	}
}
