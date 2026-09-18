package types

import ierr "github.com/flexprice/flexprice/internal/errors"

// RecognitionMethod is how a revenue_facts row's amount is recognized over
// time under ASC 606. Reserved for the Phase-4 recognition engine; left unset
// (nil) on billed-only rows in the current slice.
type RecognitionMethod string

const (
	// RecognitionMethodPointInTime recognizes the full amount on a single day.
	RecognitionMethodPointInTime RecognitionMethod = "point_in_time"
	// RecognitionMethodRatable recognizes straight-line over the service window.
	RecognitionMethodRatable RecognitionMethod = "ratable"
	// RecognitionMethodUsage recognizes as usage is delivered.
	RecognitionMethodUsage RecognitionMethod = "usage"
)

func (m RecognitionMethod) Validate() error {
	switch m {
	case RecognitionMethodPointInTime, RecognitionMethodRatable, RecognitionMethodUsage:
		return nil
	default:
		return ierr.NewErrorf("invalid recognition method %q", m).
			WithHint("recognition_method must be one of: point_in_time, ratable, usage").
			Mark(ierr.ErrValidation)
	}
}
