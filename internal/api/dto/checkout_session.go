package dto

import (
	"context"
	"time"

	domainCheckout "github.com/flexprice/flexprice/internal/domain/checkout"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
	"github.com/samber/lo"
)

// PaymentParams groups payment-provider settings for checkout flows.
type PaymentParams struct {
	PaymentProvider       types.CheckoutPaymentProvider        `json:"payment_provider" binding:"required" validate:"required"`
	PaymentProviderConfig *types.CheckoutPaymentProviderConfig `json:"payment_provider_config,omitempty"`
}

func (p *PaymentParams) Validate() error {
	if p == nil {
		return nil
	}
	if err := validator.ValidateRequest(p); err != nil {
		return err
	}
	if err := p.PaymentProvider.Validate(); err != nil {
		return err
	}
	if p.PaymentProviderConfig != nil {
		if err := p.PaymentProviderConfig.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// RedirectionParams groups post-checkout redirect URLs.
type RedirectionParams struct {
	SuccessURL *string `json:"success_url,omitempty"`
	FailureURL *string `json:"failure_url,omitempty"`
	CancelURL  *string `json:"cancel_url,omitempty"`
}

// CheckoutParams is the reusable checkout opt-in payload shared by
// create-session, payment-gated subscription modify, and wallet top-up.
type CheckoutParams struct {
	PaymentParams
	RedirectionParams
	IdempotencyKey *string           `json:"idempotency_key,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

func (p *CheckoutParams) Validate() error {
	if p == nil {
		return nil
	}
	return p.PaymentParams.Validate()
}

// ToDomainCheckoutParams maps API checkout params onto the domain builder input.
func (p *CheckoutParams) ToDomainCheckoutParams() *domainCheckout.CheckoutParams {
	if p == nil {
		return nil
	}

	var meta types.Metadata
	if len(p.Metadata) > 0 {
		meta = types.Metadata(p.Metadata)
	}
	
	return &domainCheckout.CheckoutParams{
		PaymentProvider:       p.PaymentProvider,
		PaymentProviderConfig: p.PaymentProviderConfig,
		IdempotencyKey:        p.IdempotencyKey,
		SuccessURL:            p.SuccessURL,
		FailureURL:            p.FailureURL,
		CancelURL:             p.CancelURL,
		Metadata:              meta,
	}
}

// CreateCheckoutSessionRequest is the request body for POST /checkout/sessions.
type CreateCheckoutSessionRequest struct {
	CustomerExternalID string                      `json:"customer_external_id" binding:"required"`
	Action             types.CheckoutAction        `json:"action" binding:"required"`
	Configuration      types.CheckoutConfiguration `json:"configuration"`
	CheckoutParams
}

func (r *CreateCheckoutSessionRequest) Validate() error {
	if err := validator.ValidateRequest(r); err != nil {
		return err
	}

	if err := r.Action.Validate(); err != nil {
		return err
	}

	// modify_subscription sessions are created only via subscription modify/execute (pay-first).
	if r.Action == types.CheckoutActionModifySubscription {
		return ierr.NewError("modify_subscription is not supported via create checkout session").
			WithHint("Use POST /subscriptions/{id}/modify/execute with a checkout object instead").
			Mark(ierr.ErrValidation)
	}

	// wallet_topup sessions are created only via wallet top-up (pay-first).
	if r.Action == types.CheckoutActionWalletTopup {
		return ierr.NewError("wallet_topup is not supported via create checkout session").
			WithHint("Use POST /wallets/{id}/top-up with a checkout object instead").
			Mark(ierr.ErrValidation)
	}

	if err := r.CheckoutParams.Validate(); err != nil {
		return err
	}

	if err := r.Configuration.Validate(r.Action); err != nil {
		return err
	}

	return nil
}

func (r *CreateCheckoutSessionRequest) ToCheckoutSession(ctx context.Context, customerID string) *domainCheckout.CheckoutSession {
	return domainCheckout.NewCheckoutSessionBuilder(ctx, nil).
		WithCustomerID(customerID).
		WithAction(r.Action).
		WithConfiguration(domainCheckout.ToJSONBCheckoutConfiguration(r.Configuration)).
		WithCheckoutParams(r.CheckoutParams.ToDomainCheckoutParams()).
		Build()
}

// UpdateCheckoutSessionRequest carries lifecycle-only patch fields.
// Only non-nil fields are applied.
type UpdateCheckoutSessionRequest struct {
	CheckoutStatus    *types.CheckoutStatus         `json:"checkout_status,omitempty"`
	CheckoutInvoiceID *string                       `json:"checkout_invoice_id,omitempty"`
	CheckoutPaymentID *string                       `json:"checkout_payment_id,omitempty"`
	Result            *types.CheckoutResult         `json:"result,omitempty"`
	ProviderResult    *types.CheckoutProviderResult `json:"provider_result,omitempty"`
	CompletedAt       *time.Time                    `json:"completed_at,omitempty"`
	CancelledAt       *time.Time                    `json:"cancelled_at,omitempty"`
	FailureReason     *string                       `json:"failure_reason,omitempty"`
}

// CreateCheckoutPaymentRequest holds parameters for creating an INITIATED payment
// record during checkout fulfillment. Uses the domain invoice directly to avoid
// a redundant DB lookup. Extend this struct to add metadata, idempotency keys,
// or additional gateway fields without changing the service interface signature.
type CreateCheckoutPaymentRequest struct {
	Invoice *invoice.Invoice
	Gateway types.PaymentGatewayType
}

// PayFirstCheckoutRequest is the shared settlement input for payment-gated flows.
// Callers create domain intent + DRAFT invoice after guarding concurrent pending
// sessions; StartPayFirstCheckoutSession owns session create, fulfill, cleanup,
// and initiated webhook.
type PayFirstCheckoutRequest struct {
	CustomerID    string
	Action        types.CheckoutAction
	Configuration types.CheckoutConfiguration
	DraftInvoice  *invoice.Invoice
	Checkout      *CheckoutParams
}

// ValidateCheckoutSessionForCompletion ensures a session has action params and
// locked invoice/payment IDs before completion handlers run.
func ValidateCheckoutSessionForCompletion(session *domainCheckout.CheckoutSession) error {
	if session == nil {
		return ierr.NewError("checkout session is required").
			Mark(ierr.ErrValidation)
	}
	if session.CheckoutInvoiceID == nil || *session.CheckoutInvoiceID == "" {
		return ierr.NewError("session has no checkout invoice").
			WithHint("checkout session must have checkout_invoice_id before it can be completed").
			Mark(ierr.ErrValidation)
	}
	if session.CheckoutPaymentID == nil || *session.CheckoutPaymentID == "" {
		return ierr.NewError("session has no checkout payment").
			WithHint("checkout session must have checkout_payment_id before it can be completed").
			Mark(ierr.ErrValidation)
	}

	cfg := session.Configuration.ToCheckoutConfiguration()
	switch session.Action {
	case types.CheckoutActionModifySubscription:
		if cfg.ModifySubscriptionParams == nil {
			return ierr.NewError("session has no modify_subscription_params").
				WithHint("checkout session must have modify_subscription_params before it can be completed").
				Mark(ierr.ErrValidation)
		}
		return cfg.ModifySubscriptionParams.Validate()
	case types.CheckoutActionWalletTopup:
		if cfg.WalletTopupParams == nil {
			return ierr.NewError("session has no wallet_topup_params").
				WithHint("checkout session must have wallet_topup_params before it can be completed").
				Mark(ierr.ErrValidation)
		}
		return cfg.WalletTopupParams.Validate()
	default:
		return nil
	}
}

// CheckoutSessionResponse is the API response for a single checkout session.
type CheckoutSessionResponse struct {
	*domainCheckout.CheckoutSession
	PaymentAction *types.PaymentAction `json:"payment_action,omitempty"`
}

// ListCheckoutSessionsResponse is the paginated list response.
type ListCheckoutSessionsResponse = types.ListResponse[*CheckoutSessionResponse]

// ToCheckoutSessionResponse maps a domain session to its API response.
// PaymentAction is derived from ProviderResult; the raw ProviderResult is omitted
// from the response because it contains sensitive gateway tokens.
func ToCheckoutSessionResponse(s *domainCheckout.CheckoutSession) *CheckoutSessionResponse {
	session := lo.FromPtr(s)
	paymentAction := session.ProviderResult.ToProviderResult().PaymentAction()
	session.ProviderResult = nil
	session.Result = nil
	session.Configuration = domainCheckout.JSONBCheckoutConfiguration{}
	session.PaymentProviderConfig = nil
	return &CheckoutSessionResponse{
		CheckoutSession: lo.ToPtr(session),
		PaymentAction:   paymentAction,
	}
}
