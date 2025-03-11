package types

import (
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// BillingImpactDetails provides detailed information about the financial impact of subscription actions
type BillingImpactDetails struct {
	// The amount that will be adjusted for the current period
	// Positive value indicates a charge to the customer
	// Negative value indicates a credit to the customer
	PeriodAdjustmentAmount decimal.Decimal `json:"period_adjustment_amount,omitempty"`

	// The date when the next invoice will be generated
	// For paused subscriptions, this will be after the pause ends
	NextBillingDate *time.Time `json:"next_billing_date,omitempty"`

	// The amount that will be charged on the next billing date
	// This may be prorated if resuming mid-period
	NextBillingAmount decimal.Decimal `json:"next_billing_amount,omitempty"`

	// The original billing cycle dates before pause
	OriginalPeriodStart *time.Time `json:"original_period_start,omitempty"`
	OriginalPeriodEnd   *time.Time `json:"original_period_end,omitempty"`

	// The adjusted billing cycle dates after pause
	AdjustedPeriodStart *time.Time `json:"adjusted_period_start,omitempty"`
	AdjustedPeriodEnd   *time.Time `json:"adjusted_period_end,omitempty"`

	// The total pause duration in days
	PauseDurationDays int `json:"pause_duration_days,omitempty"`
}

// InvoiceReferencePoint indicates the point in time relative to a billing period
// that determines which charges to include in an invoice
type InvoiceReferencePoint string

const (
	// ReferencePointPeriodStart indicates invoice creation at the beginning of a period (for advance charges)
	ReferencePointPeriodStart InvoiceReferencePoint = "period_start"
	// ReferencePointPeriodEnd indicates invoice creation at the end of a period (for arrear charges)
	ReferencePointPeriodEnd InvoiceReferencePoint = "period_end"
	// ReferencePointPreview indicates a preview invoice that should include all charges
	ReferencePointPreview InvoiceReferencePoint = "preview"
)

func (r InvoiceReferencePoint) String() string {
	return string(r)
}

func (r InvoiceReferencePoint) Validate() error {
	allowedValues := []InvoiceReferencePoint{
		ReferencePointPeriodStart,
		ReferencePointPeriodEnd,
		ReferencePointPreview,
	}

	if !lo.Contains(allowedValues, r) {
		return ierr.NewError("invalid invoice reference point").
			WithHint("Invalid invoice reference point").
			WithReportableDetails(map[string]any{
				"allowed_values": allowedValues,
				"provided_value": r,
			}).
			Mark(ierr.ErrValidation)
	}

	return nil
}
