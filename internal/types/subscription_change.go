package types

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/samber/lo"
)

type EntityDisposition string

const (
	EntityDispositionCarry EntityDisposition = "carry"
	EntityDispositionDrop  EntityDisposition = "drop"
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
