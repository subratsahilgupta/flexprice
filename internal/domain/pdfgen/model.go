package pdfgen

import (
	"time"

	"github.com/shopspring/decimal"
)

// InvoiceData represents the data model for invoice PDF generation
type InvoiceData struct {
	ID              string          `json:"ID"`
	InvoiceNumber   string          `json:"InvoiceNumber"`
	CustomerID      string          `json:"CustomerID"`
	SubscriptionID  string          `json:"SubscriptionID,omitempty"`
	InvoiceType     string          `json:"InvoiceType"`
	InvoiceStatus   string          `json:"InvoiceStatus"`
	PaymentStatus   string          `json:"PaymentStatus"`
	Currency        string          `json:"Currency"`
	AmountDue       decimal.Decimal `json:"AmountDue"`
	AmountPaid      decimal.Decimal `json:"AmountPaid"`
	AmountRemaining decimal.Decimal `json:"AmountRemaining"`
	Description     string          `json:"Description,omitempty"`
	DueDate         time.Time       `json:"DueDate,omitempty"`
	PaidAt          *time.Time      `json:"PaidAt,omitempty"`
	VoidedAt        *time.Time      `json:"VoidedAt,omitempty"`
	FinalizedAt     *time.Time      `json:"FinalizedAt,omitempty"`
	PeriodStart     *time.Time      `json:"PeriodStart,omitempty"`
	PeriodEnd       *time.Time      `json:"PeriodEnd,omitempty"`
	InvoicePdfURL   string          `json:"InvoicePdfURL,omitempty"`
	BillingReason   string          `json:"BillingReason,omitempty"`
	Notes           string          `json:"Notes,omitempty"`
	VAT             float64         `json:"VAT"` // VAT percentage as decimal (0.18 = 18%)

	// Company information
	Biller    *BillerInfo    `json:"biller,omitempty"`
	Recipient *RecipientInfo `json:"recipient,omitempty"`

	// Line items
	LineItems []LineItemData `json:"line_items"`
}

// BillerInfo contains company information for the invoice issuer
type BillerInfo struct {
	Name                string      `json:"Name"`
	Email               string      `json:"Email,omitempty"`
	Website             string      `json:"Website,omitempty"`
	HelpEmail           string      `json:"HelpEmail,omitempty"`
	PaymentInstructions string      `json:"PaymentInstructions,omitempty"`
	Address             AddressInfo `json:"Address"`
}

// RecipientInfo contains customer information for the invoice recipient
type RecipientInfo struct {
	Name    string      `json:"Name"`
	Email   string      `json:"Email,omitempty"`
	Address AddressInfo `json:"Address"`
}

// AddressInfo represents a physical address
type AddressInfo struct {
	Street     string `json:"Street"`
	City       string `json:"City"`
	State      string `json:"State,omitempty"`
	PostalCode string `json:"PostalCode"`
	Country    string `json:"Country,omitempty"`
}

// LineItemData represents an invoice line item for PDF generation
type LineItemData struct {
	PlanDisplayName string          `json:"PlanDisplayName"`
	DisplayName     string          `json:"DisplayName"`
	PeriodStart     *time.Time      `json:"PeriodStart,omitempty"`
	PeriodEnd       *time.Time      `json:"PeriodEnd,omitempty"`
	Amount          decimal.Decimal `json:"Amount"`
	Quantity        decimal.Decimal `json:"Quantity"`
	Currency        string          `json:"Currency"`
}
