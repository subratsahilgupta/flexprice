package types

import ierr "github.com/flexprice/flexprice/internal/errors"

type WindowSize string

// Note: keep values up to date in the meter package
const (
	WindowSizeMinute WindowSize = "MINUTE"
	WindowSizeHour   WindowSize = "HOUR"
	WindowSizeDay    WindowSize = "DAY"
)

func (w WindowSize) Validate() error {
	if w == "" {
		return nil
	}

	switch w {
	case WindowSizeMinute, WindowSizeHour, WindowSizeDay:
		return nil
	default:
		return ierr.NewError("invalid window size").
			WithHint("Invalid window size").
			WithReportableDetails(
				map[string]any{
					"window_size": w,
				},
			).
			Mark(ierr.ErrValidation)
	}
}
