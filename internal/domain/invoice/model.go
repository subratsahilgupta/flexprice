package invoice

import (
	"fmt"
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Invoice represents the invoice domain model
type Invoice struct {
	ID              string                     `json:"id"`
	CustomerID      string                     `json:"customer_id"`
	SubscriptionID  *string                    `json:"subscription_id,omitempty"`
	InvoiceType     types.InvoiceType          `json:"invoice_type"`
	InvoiceStatus   types.InvoiceStatus        `json:"invoice_status"`
	PaymentStatus   types.InvoicePaymentStatus `json:"payment_status"`
	Currency        string                     `json:"currency"`
	AmountDue       decimal.Decimal            `json:"amount_due"`
	AmountPaid      decimal.Decimal            `json:"amount_paid"`
	AmountRemaining decimal.Decimal            `json:"amount_remaining"`
	Description     string                     `json:"description,omitempty"`
	DueDate         *time.Time                 `json:"due_date,omitempty"`
	PaidAt          *time.Time                 `json:"paid_at,omitempty"`
	VoidedAt        *time.Time                 `json:"voided_at,omitempty"`
	FinalizedAt     *time.Time                 `json:"finalized_at,omitempty"`
	InvoicePDFURL   *string                    `json:"invoice_pdf_url,omitempty"`
	BillingReason   string                     `json:"billing_reason,omitempty"`
	Metadata        types.Metadata             `json:"metadata,omitempty"`
	Version         int                        `json:"version"`
	types.BaseModel
}

// FromEnt converts an ent.Invoice to domain Invoice
func FromEnt(e *ent.Invoice) *Invoice {
	if e == nil {
		return nil
	}

	return &Invoice{
		ID:              e.ID,
		CustomerID:      e.CustomerID,
		SubscriptionID:  e.SubscriptionID,
		InvoiceType:     types.InvoiceType(e.InvoiceType),
		InvoiceStatus:   types.InvoiceStatus(e.InvoiceStatus),
		PaymentStatus:   types.InvoicePaymentStatus(e.PaymentStatus),
		Currency:        e.Currency,
		AmountDue:       e.AmountDue,
		AmountPaid:      e.AmountPaid,
		AmountRemaining: e.AmountRemaining,
		Description:     e.Description,
		DueDate:         e.DueDate,
		PaidAt:          e.PaidAt,
		VoidedAt:        e.VoidedAt,
		FinalizedAt:     e.FinalizedAt,
		InvoicePDFURL:   e.InvoicePdfURL,
		BillingReason:   e.BillingReason,
		Metadata:        e.Metadata,
		Version:         e.Version,
		BaseModel: types.BaseModel{
			TenantID:  e.TenantID,
			Status:    types.Status(e.Status),
			CreatedBy: e.CreatedBy,
			UpdatedBy: e.UpdatedBy,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
		},
	}
}

// Default helper methods

func (i *Invoice) GetRemainingAmount() decimal.Decimal {
	return i.AmountDue.Sub(i.AmountPaid)
}

func (i *Invoice) Validate() error {
	// amount validations
	if i.AmountDue.IsNegative() {
		return NewValidationError("amount_due", "must be non negative")
	}

	if i.AmountPaid.IsNegative() {
		return NewValidationError("amount_paid", "must be non negative")
	}

	if i.AmountPaid.GreaterThan(i.AmountDue) {
		return NewValidationError("amount_paid", "must be less than or equal to amount_due")
	}

	if i.AmountRemaining.IsNegative() {
		return NewValidationError("amount_remaining", "must be non negative")
	}

	if i.AmountRemaining.GreaterThan(i.AmountDue) {
		return NewValidationError("amount_remaining", "must be less than or equal to amount_due")
	}

	if !i.AmountPaid.Add(i.AmountRemaining).Equal(i.AmountDue) {
		return NewValidationError("amount", "amount_paid + amount_remaining must be equal to amount_due")
	}

	// Status validations
	if !i.AmountDue.IsZero() && i.AmountPaid.Equal(i.AmountDue) && i.PaymentStatus != types.InvoicePaymentStatusSucceeded {
		return NewValidationError("payment_status", fmt.Sprintf("must be %s if amount_paid is equal to amount_due", types.InvoicePaymentStatusSucceeded))
	}

	return nil
}
