package dto

import (
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// CreateInvoiceRequest represents the request to create a new invoice
type CreateInvoiceRequest struct {
	TenantID       string                     `json:"tenant_id" validate:"required"`
	CustomerID     string                     `json:"customer_id" validate:"required"`
	SubscriptionID *string                    `json:"subscription_id,omitempty"`
	WalletID       *string                    `json:"wallet_id,omitempty"`
	Currency       string                     `json:"currency" validate:"required"`
	AmountDue      decimal.Decimal            `json:"amount_due" validate:"required"`
	Description    string                     `json:"description,omitempty"`
	DueDate        *time.Time                 `json:"due_date,omitempty"`
	BillingReason  types.InvoiceBillingReason `json:"billing_reason" validate:"required"`
	Metadata       map[string]interface{}     `json:"metadata,omitempty"`
}

func (r *CreateInvoiceRequest) Validate() error {
	if r.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if r.CustomerID == "" {
		return fmt.Errorf("customer_id is required")
	}
	if r.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if r.AmountDue.IsZero() || r.AmountDue.IsNegative() {
		return fmt.Errorf("amount_due must be greater than zero")
	}
	if r.BillingReason == "" {
		return fmt.Errorf("billing_reason is required")
	}
	return nil
}

// InvoiceResponse represents the response for invoice operations
type InvoiceResponse struct {
	ID              string                 `json:"id"`
	TenantID        string                 `json:"tenant_id"`
	CustomerID      string                 `json:"customer_id"`
	SubscriptionID  *string                `json:"subscription_id,omitempty"`
	WalletID        *string                `json:"wallet_id,omitempty"`
	InvoiceStatus   types.InvoiceStatus    `json:"invoice_status"`
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
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// NewInvoiceResponse creates a new invoice response from domain invoice
func NewInvoiceResponse(inv *invoice.Invoice) *InvoiceResponse {
	if inv == nil {
		return nil
	}

	return &InvoiceResponse{
		ID:              inv.ID,
		TenantID:        inv.TenantID,
		CustomerID:      inv.CustomerID,
		SubscriptionID:  inv.SubscriptionID,
		WalletID:        inv.WalletID,
		InvoiceStatus:   inv.InvoiceStatus,
		Currency:        inv.Currency,
		AmountDue:       inv.AmountDue,
		AmountPaid:      inv.AmountPaid,
		AmountRemaining: inv.AmountRemaining,
		Description:     inv.Description,
		DueDate:         inv.DueDate,
		PaidAt:          inv.PaidAt,
		VoidedAt:        inv.VoidedAt,
		FinalizedAt:     inv.FinalizedAt,
		PaymentIntentID: inv.PaymentIntentID,
		InvoicePdfUrl:   inv.InvoicePdfUrl,
		AttemptCount:    inv.AttemptCount,
		BillingReason:   inv.BillingReason,
		Metadata:        inv.Metadata,
		Version:         inv.Version,
		CreatedAt:       inv.CreatedAt,
		UpdatedAt:       inv.UpdatedAt,
	}
}

// ListInvoicesResponse represents the response for listing invoices
type ListInvoicesResponse struct {
	Items []*InvoiceResponse `json:"items"`
	Total int                `json:"total"`
}
