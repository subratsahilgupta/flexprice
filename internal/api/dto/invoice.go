package dto

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// CreateInvoiceRequest represents the request to create a new invoice
type CreateInvoiceRequest struct {
	CustomerID     string                      `json:"customer_id" validate:"required"`
	SubscriptionID *string                     `json:"subscription_id,omitempty"`
	InvoiceType    types.InvoiceType           `json:"invoice_type" validate:"required"`
	Currency       string                      `json:"currency" validate:"required"`
	AmountDue      decimal.Decimal             `json:"amount_due" validate:"required"`
	Description    string                      `json:"description,omitempty"`
	DueDate        *time.Time                  `json:"due_date,omitempty"`
	BillingReason  types.InvoiceBillingReason  `json:"billing_reason"`
	InvoiceStatus  *types.InvoiceStatus        `json:"invoice_status,omitempty"`
	PaymentStatus  *types.InvoicePaymentStatus `json:"payment_status,omitempty"`
	AmountPaid     *decimal.Decimal            `json:"amount_paid,omitempty"`
	Metadata       map[string]interface{}      `json:"metadata,omitempty"`
}

func (r *CreateInvoiceRequest) Validate() error {
	validate := validator.New()
	if err := validate.Struct(r); err != nil {
		return err
	}

	if r.AmountDue.IsZero() || r.AmountDue.IsNegative() {
		return fmt.Errorf("amount_due must be greater than zero")
	}

	if r.InvoiceType == types.InvoiceTypeSubscription && r.SubscriptionID == nil {
		return fmt.Errorf("subscription_id is required for subscription invoice")
	}

	return nil
}

func (r *CreateInvoiceRequest) ToInvoice(ctx context.Context) (*invoice.Invoice, error) {
	invoice := &invoice.Invoice{
		ID:              uuid.New().String(),
		CustomerID:      r.CustomerID,
		SubscriptionID:  r.SubscriptionID,
		InvoiceType:     r.InvoiceType,
		Currency:        r.Currency,
		AmountDue:       r.AmountDue,
		Description:     r.Description,
		DueDate:         r.DueDate,
		BillingReason:   string(r.BillingReason),
		Metadata:        r.Metadata,
		BaseModel:       types.GetDefaultBaseModel(ctx),
		AmountRemaining: decimal.Zero,
	}

	// Default invoice status and payment status
	if r.InvoiceStatus != nil {
		invoice.InvoiceStatus = *r.InvoiceStatus
	}

	if r.PaymentStatus != nil {
		invoice.PaymentStatus = *r.PaymentStatus
	} else {
		invoice.PaymentStatus = types.InvoicePaymentStatusPending
	}

	if r.AmountPaid != nil {
		invoice.AmountPaid = *r.AmountPaid
	}
	return invoice, nil
}

// UpdateInvoicePaymentRequest represents the request to update invoice payment status
type UpdateInvoicePaymentRequest struct {
	PaymentStatus types.InvoicePaymentStatus `json:"payment_status" validate:"required"`
}

func (r *UpdateInvoicePaymentRequest) Validate() error {
	if r.PaymentStatus == "" {
		return fmt.Errorf("payment_status is required")
	}
	return nil
}

// UpdateInvoicePaymentStatusRequest represents a request to update an invoice's payment status
type UpdateInvoicePaymentStatusRequest struct {
	PaymentStatus types.InvoicePaymentStatus `json:"payment_status" binding:"required"`
	Amount        *decimal.Decimal           `json:"amount,omitempty"`
}

func (r *UpdateInvoicePaymentStatusRequest) Validate() error {
	if r.Amount != nil && r.Amount.IsNegative() {
		return fmt.Errorf("amount must be non-negative")
	}
	return nil
}

// InvoiceResponse represents the response for invoice operations
type InvoiceResponse struct {
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
	Metadata        map[string]interface{}     `json:"metadata,omitempty"`
	Version         int                        `json:"version"`
	TenantID        string                     `json:"tenant_id"`
	Status          string                     `json:"status"`
	CreatedAt       time.Time                  `json:"created_at"`
	UpdatedAt       time.Time                  `json:"updated_at"`
	CreatedBy       string                     `json:"created_by,omitempty"`
	UpdatedBy       string                     `json:"updated_by,omitempty"`
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
		InvoiceType:     inv.InvoiceType,
		InvoiceStatus:   inv.InvoiceStatus,
		PaymentStatus:   inv.PaymentStatus,
		Currency:        inv.Currency,
		AmountDue:       inv.AmountDue,
		AmountPaid:      inv.AmountPaid,
		AmountRemaining: inv.AmountRemaining,
		Description:     inv.Description,
		DueDate:         inv.DueDate,
		PaidAt:          inv.PaidAt,
		VoidedAt:        inv.VoidedAt,
		FinalizedAt:     inv.FinalizedAt,
		InvoicePDFURL:   inv.InvoicePDFURL,
		BillingReason:   inv.BillingReason,
		Metadata:        inv.Metadata,
		Version:         inv.Version,
		Status:          string(inv.Status),
		CreatedAt:       inv.CreatedAt,
		UpdatedAt:       inv.UpdatedAt,
		CreatedBy:       inv.CreatedBy,
		UpdatedBy:       inv.UpdatedBy,
	}
}

// ListInvoicesResponse represents the response for listing invoices
type ListInvoicesResponse struct {
	Items []*InvoiceResponse `json:"items"`
	Total int                `json:"total"`
}
