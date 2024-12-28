package invoice

import (
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Invoice represents the invoice domain model
type Invoice struct {
	ID              string                 `json:"id"`
	CustomerID      string                 `json:"customer_id"`
	SubscriptionID  *string                `json:"subscription_id,omitempty"`
	WalletID        *string                `json:"wallet_id,omitempty"`
	InvoiceStatus   types.InvoiceStatus    `json:"status"`
	Currency        string                 `json:"currency"`
	AmountDue       decimal.Decimal        `json:"amount_due"`
	AmountPaid      decimal.Decimal        `json:"amount_paid"`
	AmountRemaining decimal.Decimal        `json:"amount_remaining"`
	Description     string                 `json:"description,omitempty"`
	DueDate         *time.Time             `json:"due_date,omitempty"`
	PaidAt          *time.Time             `json:"paid_at,omitempty"`
	VoidedAt        *time.Time             `json:"voided_at,omitempty"`
	FinalizedAt     *time.Time             `json:"finalized_at,omitempty"`
	PaymentIntentID *string                `json:"payment_intent_id,omitempty"`
	InvoicePdfUrl   *string                `json:"invoice_pdf_url,omitempty"`
	AttemptCount    int                    `json:"attempt_count"`
	BillingReason   string                 `json:"billing_reason,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	Version         int                    `json:"version"`
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
		WalletID:        e.WalletID,
		InvoiceStatus:   types.InvoiceStatus(e.Status),
		Currency:        e.Currency,
		AmountDue:       e.AmountDue,
		AmountPaid:      e.AmountPaid,
		AmountRemaining: e.AmountRemaining,
		Description:     e.Description,
		DueDate:         e.DueDate,
		PaidAt:          e.PaidAt,
		VoidedAt:        e.VoidedAt,
		FinalizedAt:     e.FinalizedAt,
		PaymentIntentID: e.PaymentIntentID,
		InvoicePdfUrl:   e.InvoicePdfURL,
		AttemptCount:    e.AttemptCount,
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
