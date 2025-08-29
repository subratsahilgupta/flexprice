package dto

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
	"github.com/shopspring/decimal"
)

// SubscriptionChangeRequest represents the request to change a subscription plan
// @Description Request object for changing a subscription plan (upgrade/downgrade)
type SubscriptionChangeRequest struct {
	// target_plan_id is the ID of the new plan to change to (required)
	TargetPlanID string `json:"target_plan_id" validate:"required" binding:"required"`

	// proration_behavior controls how proration is handled for the change
	// Options: create_prorations, none
	ProrationBehavior types.ProrationBehavior `json:"proration_behavior" validate:"required" binding:"required"`

	// effective_date is when the change should take effect (optional, defaults to now)
	EffectiveDate *time.Time `json:"effective_date,omitempty"`

	// billing_cycle_anchor controls how the billing cycle is handled after the change
	// Options: unchanged, reset, immediate
	BillingCycleAnchor types.BillingCycleAnchor `json:"billing_cycle_anchor,omitempty"`

	// metadata contains additional key-value pairs for storing extra information
	Metadata map[string]string `json:"metadata,omitempty"`

	// trial_end allows setting a new trial end date (optional)
	TrialEnd *time.Time `json:"trial_end,omitempty"`

	// cancel_at_period_end if true, schedules cancellation at the end of current period
	CancelAtPeriodEnd *bool `json:"cancel_at_period_end,omitempty"`

	// invoice_now controls whether to generate an invoice immediately for the change
	InvoiceNow *bool `json:"invoice_now,omitempty"`
}

// Validate validates the subscription change request
func (r *SubscriptionChangeRequest) Validate() error {
	// Validate using struct tags first
	if err := validator.ValidateRequest(r); err != nil {
		return err
	}

	// Validate proration behavior
	if err := r.ProrationBehavior.Validate(); err != nil {
		return err
	}

	// Validate billing cycle anchor if provided
	if r.BillingCycleAnchor != "" {
		if err := r.BillingCycleAnchor.Validate(); err != nil {
			return err
		}
	}

	// Validate effective date is not in the past (allow up to 1 minute grace period)
	if r.EffectiveDate != nil && r.EffectiveDate.Before(time.Now().Add(-time.Minute)) {
		return ierr.NewError("effective_date cannot be in the past").
			WithHint("Effective date must be current time or future").
			Mark(ierr.ErrValidation)
	}

	// Validate trial end is in the future if provided
	if r.TrialEnd != nil && r.TrialEnd.Before(time.Now()) {
		return ierr.NewError("trial_end must be in the future").
			WithHint("Trial end date must be in the future").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// SubscriptionChangePreviewRequest represents a request to preview subscription changes
// @Description Request object for previewing the effects of a subscription plan change
type SubscriptionChangePreviewRequest struct {
	SubscriptionChangeRequest

	// preview_date is the date to calculate the preview for (optional, defaults to effective_date or now)
	PreviewDate *time.Time `json:"preview_date,omitempty"`
}

// SubscriptionChangePreviewResponse represents the preview of subscription changes
// @Description Response showing the financial impact of a subscription plan change
type SubscriptionChangePreviewResponse struct {
	// subscription_id is the ID of the subscription being changed
	SubscriptionID string `json:"subscription_id"`

	// current_plan contains information about the current plan
	CurrentPlan PlanSummary `json:"current_plan"`

	// target_plan contains information about the target plan
	TargetPlan PlanSummary `json:"target_plan"`

	// change_type indicates whether this is an upgrade, downgrade, or lateral change
	ChangeType types.SubscriptionChangeType `json:"change_type"`

	// proration_details contains the calculated proration amounts
	ProrationDetails *ProrationDetails `json:"proration_details,omitempty"`

	// immediate_invoice_preview shows what would be invoiced immediately
	ImmediateInvoicePreview *InvoicePreview `json:"immediate_invoice_preview,omitempty"`

	// next_invoice_preview shows how the next regular invoice would be affected
	NextInvoicePreview *InvoicePreview `json:"next_invoice_preview,omitempty"`

	// effective_date is when the change would take effect
	EffectiveDate time.Time `json:"effective_date"`

	// new_billing_cycle shows the new billing cycle details
	NewBillingCycle BillingCycleInfo `json:"new_billing_cycle"`

	// warnings contains any warnings about the change
	Warnings []string `json:"warnings,omitempty"`

	// metadata from the request
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SubscriptionChangeExecuteResponse represents the result of executing a subscription change
// @Description Response after successfully executing a subscription plan change
type SubscriptionChangeExecuteResponse struct {
	// old_subscription contains the archived subscription details
	OldSubscription SubscriptionSummary `json:"old_subscription"`

	// new_subscription contains the new subscription details
	NewSubscription SubscriptionSummary `json:"new_subscription"`

	// change_type indicates whether this was an upgrade, downgrade, or lateral change
	ChangeType types.SubscriptionChangeType `json:"change_type"`

	// invoice contains the immediate invoice generated for the change (if any)
	Invoice *InvoiceResponse `json:"invoice,omitempty"`

	// proration_applied contains details of the proration that was applied
	ProrationApplied *ProrationDetails `json:"proration_applied,omitempty"`

	// effective_date is when the change took effect
	EffectiveDate time.Time `json:"effective_date"`

	// metadata from the request
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ProrationDetails contains detailed proration calculations
type ProrationDetails struct {
	// credit_amount is the credit amount from the old subscription
	CreditAmount decimal.Decimal `json:"credit_amount"`

	// credit_description describes what the credit is for
	CreditDescription string `json:"credit_description"`

	// charge_amount is the charge amount for the new subscription
	ChargeAmount decimal.Decimal `json:"charge_amount"`

	// charge_description describes what the charge is for
	ChargeDescription string `json:"charge_description"`

	// net_amount is the net amount (charge - credit)
	NetAmount decimal.Decimal `json:"net_amount"`

	// proration_date is the date used for proration calculations
	ProrationDate time.Time `json:"proration_date"`

	// current_period_start is the start of the current billing period
	CurrentPeriodStart time.Time `json:"current_period_start"`

	// current_period_end is the end of the current billing period
	CurrentPeriodEnd time.Time `json:"current_period_end"`

	// days_used is the number of days used in the current period
	DaysUsed int `json:"days_used"`

	// days_remaining is the number of days remaining in the current period
	DaysRemaining int `json:"days_remaining"`

	// currency is the currency for all amounts
	Currency string `json:"currency"`
}

// InvoicePreview contains preview information for an invoice
type InvoicePreview struct {
	// subtotal is the subtotal amount before taxes
	Subtotal decimal.Decimal `json:"subtotal"`

	// tax_amount is the total tax amount
	TaxAmount decimal.Decimal `json:"tax_amount"`

	// total is the total amount including taxes
	Total decimal.Decimal `json:"total"`

	// currency is the currency for all amounts
	Currency string `json:"currency"`

	// line_items contains preview of line items
	LineItems []InvoiceLineItemPreview `json:"line_items"`

	// due_date is when the invoice would be due
	DueDate *time.Time `json:"due_date,omitempty"`
}

// InvoiceLineItemPreview contains preview information for an invoice line item
type InvoiceLineItemPreview struct {
	// description of the line item
	Description string `json:"description"`

	// amount for this line item
	Amount decimal.Decimal `json:"amount"`

	// quantity for this line item
	Quantity decimal.Decimal `json:"quantity"`

	// unit_price for this line item
	UnitPrice decimal.Decimal `json:"unit_price"`

	// period_start for this line item (if applicable)
	PeriodStart *time.Time `json:"period_start,omitempty"`

	// period_end for this line item (if applicable)
	PeriodEnd *time.Time `json:"period_end,omitempty"`

	// is_proration indicates if this line item is a proration
	IsProration bool `json:"is_proration"`
}

// PlanSummary contains summary information about a plan
type PlanSummary struct {
	// id of the plan
	ID string `json:"id"`

	// name of the plan
	Name string `json:"name"`

	// lookup_key of the plan
	LookupKey string `json:"lookup_key,omitempty"`

	// description of the plan
	Description string `json:"description,omitempty"`
}

// SubscriptionSummary contains summary information about a subscription
type SubscriptionSummary struct {
	// id of the subscription
	ID string `json:"id"`

	// status of the subscription
	Status types.SubscriptionStatus `json:"status"`

	// plan_id of the subscription
	PlanID string `json:"plan_id"`

	// current_period_start of the subscription
	CurrentPeriodStart time.Time `json:"current_period_start"`

	// current_period_end of the subscription
	CurrentPeriodEnd time.Time `json:"current_period_end"`

	// billing_anchor of the subscription
	BillingAnchor time.Time `json:"billing_anchor"`

	// created_at timestamp
	CreatedAt time.Time `json:"created_at"`

	// archived_at timestamp (for old subscriptions)
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// BillingCycleInfo contains information about billing cycle
type BillingCycleInfo struct {
	// period_start is the start of the new billing period
	PeriodStart time.Time `json:"period_start"`

	// period_end is the end of the new billing period
	PeriodEnd time.Time `json:"period_end"`

	// billing_anchor is the new billing anchor
	BillingAnchor time.Time `json:"billing_anchor"`

	// billing_cadence is the billing cadence
	BillingCadence types.BillingCadence `json:"billing_cadence"`

	// billing_period is the billing period
	BillingPeriod types.BillingPeriod `json:"billing_period"`

	// billing_period_count is the billing period count
	BillingPeriodCount int `json:"billing_period_count"`
}

// ToSubscriptionChange converts the request to a domain subscription change
func (r *SubscriptionChangeRequest) ToSubscriptionChange(ctx context.Context, subscriptionID string) *subscription.SubscriptionChange {
	effectiveDate := time.Now()
	if r.EffectiveDate != nil {
		effectiveDate = *r.EffectiveDate
	}

	billingCycleAnchor := types.BillingCycleAnchorUnchanged
	if r.BillingCycleAnchor != "" {
		billingCycleAnchor = r.BillingCycleAnchor
	}

	invoiceNow := true
	if r.InvoiceNow != nil {
		invoiceNow = *r.InvoiceNow
	}

	return &subscription.SubscriptionChange{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_CHANGE),
		SubscriptionID:     subscriptionID,
		TargetPlanID:       r.TargetPlanID,
		ProrationBehavior:  r.ProrationBehavior,
		EffectiveDate:      effectiveDate,
		BillingCycleAnchor: billingCycleAnchor,
		TrialEnd:           r.TrialEnd,
		CancelAtPeriodEnd:  r.CancelAtPeriodEnd,
		InvoiceNow:         invoiceNow,
		Metadata:           types.Metadata(r.Metadata),
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
}
