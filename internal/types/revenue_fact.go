package types

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
)

// RevenueSource classifies what generated a revenue_facts row.
type RevenueSource string

const (
	RevenueSourceUsage            RevenueSource = "usage"
	RevenueSourceFixed            RevenueSource = "fixed"
	RevenueSourceCommitmentTrueup RevenueSource = "commitment_trueup"
)

func (s RevenueSource) Validate() error {
	switch s {
	case RevenueSourceUsage, RevenueSourceFixed, RevenueSourceCommitmentTrueup:
		return nil
	default:
		return ierr.NewErrorf("invalid revenue source %q", s).
			WithHint("revenue source must be one of: usage, fixed, commitment_trueup").
			Mark(ierr.ErrValidation)
	}
}

// DecompositionMode indicates how a revenue_facts row's amount was split across days.
type DecompositionMode string

const (
	Marginal   DecompositionMode = "marginal"
	PeriodOnly DecompositionMode = "period_only"
)

func (m DecompositionMode) Validate() error {
	switch m {
	case Marginal, PeriodOnly:
		return nil
	default:
		return ierr.NewErrorf("invalid decomposition mode %q", m).
			WithHint("decomposition mode must be one of: marginal, period_only").
			Mark(ierr.ErrValidation)
	}
}

// FactStatus is the lifecycle status of a revenue_facts row.
type FactStatus string

const (
	FactProvisional FactStatus = "PROVISIONAL"
	FactFinal       FactStatus = "FINAL"
)

func (s FactStatus) Validate() error {
	switch s {
	case FactProvisional, FactFinal:
		return nil
	default:
		return ierr.NewErrorf("invalid fact status %q", s).
			WithHint("fact status must be one of: PROVISIONAL, FINAL").
			Mark(ierr.ErrValidation)
	}
}
