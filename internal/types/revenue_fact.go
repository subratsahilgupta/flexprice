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
	RevenueSourceOverage          RevenueSource = "overage"
)

func (s RevenueSource) Validate() error {
	switch s {
	case RevenueSourceUsage, RevenueSourceFixed, RevenueSourceCommitmentTrueup, RevenueSourceOverage:
		return nil
	default:
		return ierr.NewErrorf("invalid revenue source %q", s).
			WithHint("revenue source must be one of: usage, fixed, commitment_trueup, overage").
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

// RevenueGranularity buckets revenue analytics by day, billing period, or one total.
type RevenueGranularity string

const (
	RevenueGranularityDay    RevenueGranularity = "day"
	RevenueGranularityPeriod RevenueGranularity = "period"
	RevenueGranularityTotal  RevenueGranularity = "total"
)

func (g RevenueGranularity) Validate() error {
	switch g {
	case RevenueGranularityDay, RevenueGranularityPeriod, RevenueGranularityTotal:
		return nil
	default:
		return ierr.NewErrorf("invalid revenue granularity %q", g).
			WithHint("granularity must be one of: day, period, total").
			Mark(ierr.ErrValidation)
	}
}

// RevenueAllocationPolicy controls how whole-period amounts appear in the day
// view: on their booked day, or spread evenly across their period.
type RevenueAllocationPolicy string

const (
	RevenueAllocationBilled    RevenueAllocationPolicy = "billed"
	RevenueAllocationAmortized RevenueAllocationPolicy = "amortized"
)

func (p RevenueAllocationPolicy) Validate() error {
	switch p {
	case RevenueAllocationBilled, RevenueAllocationAmortized:
		return nil
	default:
		return ierr.NewErrorf("invalid allocation policy %q", p).
			WithHint("allocation_policy must be one of: billed, amortized").
			Mark(ierr.ErrValidation)
	}
}
