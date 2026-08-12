package types

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/samber/lo"
)

// EntityDisposition says what a plan change does to something already attached
// to the subscription. v0 applies it to addons; the vocabulary is deliberately
// entity-agnostic so coupons, tax associations and credit grants can reuse it
// without inventing a second one.
type EntityDisposition string

const (
	// EntityDispositionCarry leaves the attachment untouched — zero operations.
	// The default for everything, because a swap-in-place plan change does not
	// disturb anything keyed on the subscription id.
	EntityDispositionCarry EntityDisposition = "carry"

	// EntityDispositionDrop closes the attachment at the change's effective
	// date, settling money per the change's proration_behavior.
	EntityDispositionDrop EntityDisposition = "drop"
)

var EntityDispositionValues = []EntityDisposition{
	EntityDispositionCarry,
	EntityDispositionDrop,
}

func (d EntityDisposition) String() string { return string(d) }

func (d EntityDisposition) Validate() error {
	if d == "" {
		return nil
	}
	if !lo.Contains(EntityDispositionValues, d) {
		return ierr.NewError("invalid entity disposition").
			WithHint("Disposition must be one of the allowed values").
			WithReportableDetails(map[string]any{
				"disposition": string(d),
				"allowed":     EntityDispositionValues,
			}).
			Mark(ierr.ErrValidation)
	}
	return nil
}
