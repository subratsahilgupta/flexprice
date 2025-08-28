package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
	"github.com/shopspring/decimal"
)

// UpgradeSubscriptionRequest represents a request to upgrade a subscription to a higher plan
type UpgradeSubscriptionRequest struct {
	// TargetPlanID is the ID of the plan to upgrade to
	TargetPlanID string `json:"target_plan_id" validate:"required"`

	// ProrationBehavior determines how proration is handled
	ProrationBehavior types.ProrationBehavior `json:"proration_behavior"`

	// EffectiveImmediately determines if the upgrade should take effect immediately
	EffectiveImmediately bool `json:"effective_immediately"`

	// Metadata for additional information about the upgrade
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DowngradeSubscriptionRequest represents a request to downgrade a subscription to a lower plan
type DowngradeSubscriptionRequest struct {
	// TargetPlanID is the ID of the plan to downgrade to
	TargetPlanID string `json:"target_plan_id" validate:"required"`

	// ProrationBehavior determines how proration is handled
	ProrationBehavior types.ProrationBehavior `json:"proration_behavior"`

	// EffectiveAtPeriodEnd determines if the downgrade should take effect at period end
	EffectiveAtPeriodEnd bool `json:"effective_at_period_end"`

	// EffectiveDate is the specific date when the downgrade should take effect (optional)
	EffectiveDate *time.Time `json:"effective_date,omitempty"`

	// Metadata for additional information about the downgrade
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PreviewPlanChangeRequest represents a request to preview a plan change
type PreviewPlanChangeRequest struct {
	// TargetPlanID is the ID of the plan to change to
	TargetPlanID string `json:"target_plan_id" validate:"required"`

	// ProrationBehavior determines how proration is handled
	ProrationBehavior types.ProrationBehavior `json:"proration_behavior"`

	// EffectiveDate is when the change would take effect (optional, defaults to now)
	EffectiveDate *time.Time `json:"effective_date,omitempty"`
}

// SubscriptionPlanChangeResult represents the result of a plan change operation
type SubscriptionPlanChangeResult struct {
	// Subscription is the updated subscription
	Subscription *subscription.Subscription `json:"subscription"`

	// Invoice is the proration invoice generated (if any)
	Invoice *InvoiceResponse `json:"invoice,omitempty"`

	// Schedule is the subscription schedule (for period-end changes)
	Schedule *SubscriptionScheduleResponse `json:"schedule,omitempty"`

	// ProrationAmount is the net proration amount
	ProrationAmount decimal.Decimal `json:"proration_amount"`

	// ChangeType indicates the type of change (upgrade, downgrade, etc.)
	ChangeType string `json:"change_type"`

	// EffectiveDate is when the change takes effect
	EffectiveDate time.Time `json:"effective_date"`

	// Metadata contains additional information about the change
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PlanChangePreviewResult represents the preview of a plan change
type PlanChangePreviewResult struct {
	// CurrentAmount is the current subscription amount
	CurrentAmount decimal.Decimal `json:"current_amount"`

	// NewAmount is the new subscription amount after change
	NewAmount decimal.Decimal `json:"new_amount"`

	// ProrationAmount is the net proration amount
	ProrationAmount decimal.Decimal `json:"proration_amount"`

	// EffectiveDate is when the change would take effect
	EffectiveDate time.Time `json:"effective_date"`

	// LineItems shows the breakdown of proration line items
	LineItems []ProrationLineItemPreview `json:"line_items"`

	// Taxes shows tax calculations (if applicable)
	Taxes *TaxCalculationPreview `json:"taxes,omitempty"`

	// Coupons shows how coupons would be affected
	Coupons []CouponImpactPreview `json:"coupons,omitempty"`
}

// ProrationLineItemPreview represents a preview of a proration line item
type ProrationLineItemPreview struct {
	// Description of the line item
	Description string `json:"description"`

	// Amount of the line item (positive for charges, negative for credits)
	Amount decimal.Decimal `json:"amount"`

	// Quantity for the line item
	Quantity decimal.Decimal `json:"quantity"`

	// PriceID associated with this line item
	PriceID string `json:"price_id"`

	// Type indicates if this is a credit or charge
	Type string `json:"type"` // "credit" or "charge"

	// Metadata for additional information
	Metadata map[string]string `json:"metadata,omitempty"`
}

// TaxCalculationPreview represents tax calculation preview
type TaxCalculationPreview struct {
	// TotalTax is the total tax amount
	TotalTax decimal.Decimal `json:"total_tax"`

	// TaxRate is the effective tax rate
	TaxRate decimal.Decimal `json:"tax_rate"`

	// TaxableAmount is the amount subject to tax
	TaxableAmount decimal.Decimal `json:"taxable_amount"`

	// Breakdown of tax by jurisdiction (if applicable)
	Breakdown []TaxBredownItem `json:"breakdown,omitempty"`
}

// TaxBredownItem represents a single tax breakdown item
type TaxBredownItem struct {
	// Jurisdiction name
	Jurisdiction string `json:"jurisdiction"`

	// Tax rate for this jurisdiction
	Rate decimal.Decimal `json:"rate"`

	// Tax amount for this jurisdiction
	Amount decimal.Decimal `json:"amount"`
}

// CouponImpactPreview represents how a coupon would be affected by the plan change
type CouponImpactPreview struct {
	// CouponID is the ID of the coupon
	CouponID string `json:"coupon_id"`

	// CouponName is the name of the coupon
	CouponName string `json:"coupon_name"`

	// Action indicates what would happen to the coupon
	Action string `json:"action"` // "keep", "deactivate", "migrate", "convert"

	// CurrentDiscount is the current discount amount
	CurrentDiscount decimal.Decimal `json:"current_discount"`

	// NewDiscount is the new discount amount (if applicable)
	NewDiscount decimal.Decimal `json:"new_discount,omitempty"`

	// Reason explains why this action would be taken
	Reason string `json:"reason"`
}

// Validation methods

// validateProrationBehavior validates the proration behavior value
func validateProrationBehavior(behavior types.ProrationBehavior) error {
	validBehaviors := []types.ProrationBehavior{
		types.ProrationBehaviorCreateProrations,
		types.ProrationBehaviorAlwaysInvoice,
		types.ProrationBehaviorNone,
	}

	for _, valid := range validBehaviors {
		if behavior == valid {
			return nil
		}
	}

	return ierr.NewErrorf("invalid proration behavior: %s", behavior).
		WithHint("Valid values are: create_prorations, always_invoice, none").
		Mark(ierr.ErrValidation)
}

func (r *UpgradeSubscriptionRequest) Validate() error {
	if err := validator.ValidateRequest(r); err != nil {
		return err
	}

	// Set default proration behavior if not specified
	if r.ProrationBehavior == "" {
		r.ProrationBehavior = types.ProrationBehaviorCreateProrations
	}

	// Validate proration behavior
	if err := validateProrationBehavior(r.ProrationBehavior); err != nil {
		return err
	}

	return nil
}

func (r *DowngradeSubscriptionRequest) Validate() error {
	if err := validator.ValidateRequest(r); err != nil {
		return err
	}

	// Set default proration behavior if not specified
	if r.ProrationBehavior == "" {
		r.ProrationBehavior = types.ProrationBehaviorCreateProrations
	}

	// Validate proration behavior
	if err := validateProrationBehavior(r.ProrationBehavior); err != nil {
		return err
	}

	// Validate effective date if provided
	if r.EffectiveDate != nil && r.EffectiveDate.Before(time.Now()) {
		return ierr.NewError("effective_date cannot be in the past").
			WithHint("Effective date must be in the future").
			Mark(ierr.ErrValidation)
	}

	return nil
}

func (r *PreviewPlanChangeRequest) Validate() error {
	if err := validator.ValidateRequest(r); err != nil {
		return err
	}

	// Set default proration behavior if not specified
	if r.ProrationBehavior == "" {
		r.ProrationBehavior = types.ProrationBehaviorCreateProrations
	}

	// Validate proration behavior
	if err := validateProrationBehavior(r.ProrationBehavior); err != nil {
		return err
	}

	// Set default effective date if not provided
	if r.EffectiveDate == nil {
		now := time.Now()
		r.EffectiveDate = &now
	}

	return nil
}
