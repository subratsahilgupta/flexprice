package dto

import (
	"strings"

	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// CreateStripePaymentLinkRequest represents a request to create a Stripe payment link
type CreateStripePaymentLinkRequest struct {
	InvoiceID              string          `json:"invoice_id" binding:"required"`
	CustomerID             string          `json:"customer_id" binding:"required"`
	Amount                 decimal.Decimal `json:"amount" binding:"required"`
	Currency               string          `json:"currency" binding:"required"`
	SuccessURL             string          `json:"success_url,omitempty"`
	CancelURL              string          `json:"cancel_url,omitempty"`
	EnvironmentID          string          `json:"environment_id" binding:"required"`
	Metadata               types.Metadata  `json:"metadata,omitempty"`
	SaveCardAndMakeDefault bool            `json:"save_card_and_make_default" default:"false"`
}

// StripePaymentLinkResponse represents a response from creating a Stripe payment link
type StripePaymentLinkResponse struct {
	ID              string          `json:"id"`
	PaymentURL      string          `json:"payment_url"`
	PaymentIntentID string          `json:"payment_intent_id"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
	Status          string          `json:"status"`
	CreatedAt       int64           `json:"created_at"`
	PaymentID       string          `json:"payment_id,omitempty"`
}

// Validate validates the create Stripe payment link request
func (r *CreateStripePaymentLinkRequest) Validate() error {
	if r.InvoiceID == "" {
		return errors.NewError("invoice_id is required").
			WithHint("Invoice ID is required").
			Mark(errors.ErrValidation)
	}

	if r.CustomerID == "" {
		return errors.NewError("customer_id is required").
			WithHint("Customer ID is required").
			Mark(errors.ErrValidation)
	}

	if r.Amount.IsZero() || r.Amount.IsNegative() {
		return errors.NewError("invalid amount").
			WithHint("Amount must be greater than 0").
			Mark(errors.ErrValidation)
	}

	if r.Currency == "" {
		return errors.NewError("currency is required").
			WithHint("Currency is required").
			Mark(errors.ErrValidation)
	}

	if err := types.ValidateCurrencyCode(r.Currency); err != nil {
		return err
	}

	if r.EnvironmentID == "" {
		return errors.NewError("environment_id is required").
			WithHint("Environment ID is required").
			Mark(errors.ErrValidation)
	}

	return nil
}

// PaymentStatusResponse represents the payment status from Stripe
type PaymentStatusResponse struct {
	SessionID       string            `json:"session_id"`
	PaymentIntentID string            `json:"payment_intent_id"`
	PaymentMethodID string            `json:"payment_method_id,omitempty"`
	Status          string            `json:"status"`
	Amount          decimal.Decimal   `json:"amount"`
	Currency        string            `json:"currency"`
	CustomerID      string            `json:"customer_id"`
	CreatedAt       int64             `json:"created_at"`
	ExpiresAt       int64             `json:"expires_at"`
	Metadata        map[string]string `json:"metadata"`
}

// CreateSetupIntentRequest represents a request to create a Setup Intent session
type CreateSetupIntentRequest struct {
	CustomerID         string         `json:"customer_id" binding:"required"`
	Usage              string         `json:"usage,omitempty"`                // "on_session" or "off_session" (default: "off_session")
	PaymentMethodTypes []string       `json:"payment_method_types,omitempty"` // defaults to ["card"]
	SuccessURL         string         `json:"success_url,omitempty"`          // User-configurable success redirect URL
	CancelURL          string         `json:"cancel_url,omitempty"`           // User-configurable cancel redirect URL
	Metadata           types.Metadata `json:"metadata,omitempty"`
}

// SetupIntentResponse represents a response from creating a Setup Intent session
type SetupIntentResponse struct {
	SetupIntentID     string `json:"setup_intent_id"`
	CheckoutSessionID string `json:"checkout_session_id"`
	CheckoutURL       string `json:"checkout_url"`
	ClientSecret      string `json:"client_secret"`
	Status            string `json:"status"`
	Usage             string `json:"usage"`
	CustomerID        string `json:"customer_id"`
	CreatedAt         int64  `json:"created_at"`
	ExpiresAt         int64  `json:"expires_at"`
}

// Validate validates the create Setup Intent request
func (r *CreateSetupIntentRequest) Validate() error {
	if r.CustomerID == "" {
		return errors.NewError("customer_id is required").
			WithHint("Customer ID is required").
			Mark(errors.ErrValidation)
	}

	// Validate usage parameter
	if r.Usage != "" && r.Usage != "on_session" && r.Usage != "off_session" {
		return errors.NewError("invalid usage parameter").
			WithHint("Usage must be 'on_session' or 'off_session'").
			Mark(errors.ErrValidation)
	}

	// Validate payment method types
	if len(r.PaymentMethodTypes) > 0 {
		for _, pmType := range r.PaymentMethodTypes {
			if pmType != "card" && pmType != "us_bank_account" && pmType != "sepa_debit" {
				return errors.NewError("unsupported payment method type").
					WithHint("Supported payment method types: card, us_bank_account, sepa_debit").
					WithReportableDetails(map[string]interface{}{
						"payment_method_type": pmType,
					}).
					Mark(errors.ErrValidation)
			}
		}
	}

	// Validate URLs if provided (basic URL format validation)
	if r.SuccessURL != "" {
		if !isValidURL(r.SuccessURL) {
			return errors.NewError("invalid success_url format").
				WithHint("Success URL must be a valid HTTP/HTTPS URL").
				WithReportableDetails(map[string]interface{}{
					"success_url": r.SuccessURL,
				}).
				Mark(errors.ErrValidation)
		}
	}

	if r.CancelURL != "" {
		if !isValidURL(r.CancelURL) {
			return errors.NewError("invalid cancel_url format").
				WithHint("Cancel URL must be a valid HTTP/HTTPS URL").
				WithReportableDetails(map[string]interface{}{
					"cancel_url": r.CancelURL,
				}).
				Mark(errors.ErrValidation)
		}
	}

	return nil
}

// isValidURL checks if a string is a valid HTTP/HTTPS URL
func isValidURL(urlStr string) bool {
	if urlStr == "" {
		return true // Empty URLs are allowed (optional)
	}

	// Must start with http:// or https://
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return false
	}

	// Basic length check
	if len(urlStr) > 2048 {
		return false // URLs longer than 2048 chars are generally not supported
	}

	return true
}
