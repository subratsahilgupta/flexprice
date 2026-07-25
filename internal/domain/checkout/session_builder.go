package checkout

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// CheckoutParams is the payment/redirect slice applied when creating a session.
// Mirrors the shared checkout opt-in payload from the API layer.
type CheckoutParams struct {
	PaymentProvider       types.CheckoutPaymentProvider
	PaymentProviderConfig *types.CheckoutPaymentProviderConfig
	IdempotencyKey        *string
	SuccessURL            *string
	FailureURL            *string
	CancelURL             *string
	Metadata              types.Metadata
}

// checkoutSessionBuilder copies an existing session and applies field updates.
type checkoutSessionBuilder struct {
	session *CheckoutSession
}

// NewCheckoutSessionBuilder returns a builder seeded from ctx create defaults
// (ID, environment_id, initiated status, base model) when session is nil, or
// from a copy of an existing session for updates. Those create-time fields are
// not mutable via With* methods.
func NewCheckoutSessionBuilder(ctx context.Context, session *CheckoutSession) *checkoutSessionBuilder {
	if session == nil {
		return nil
	}

	copied := *session
	if session.Metadata != nil {
		copied.Metadata = make(types.Metadata, len(session.Metadata))
		for k, v := range session.Metadata {
			copied.Metadata[k] = v
		}
	}
	return &checkoutSessionBuilder{session: &copied}
}

func (b *checkoutSessionBuilder) InitiateSession(ctx context.Context) *checkoutSessionBuilder {
	if b == nil || b.session == nil {
		return b
	}
	b.session.CheckoutStatus = types.CheckoutStatusInitiated
	b.session.ID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CHECKOUT_SESSION)
	b.session.EnvironmentID = types.GetEnvironmentID(ctx)
	b.session.BaseModel = types.GetDefaultBaseModel(ctx)

	return b
}

func (b *checkoutSessionBuilder) WithCustomerID(customerID string) *checkoutSessionBuilder {
	if b == nil || b.session == nil {
		return b
	}
	b.session.CustomerID = customerID
	return b
}

func (b *checkoutSessionBuilder) WithAction(action types.CheckoutAction) *checkoutSessionBuilder {
	if b == nil || b.session == nil {
		return b
	}
	b.session.Action = action
	return b
}

func (b *checkoutSessionBuilder) WithConfiguration(cfg JSONBCheckoutConfiguration) *checkoutSessionBuilder {
	if b == nil || b.session == nil {
		return b
	}
	b.session.Configuration = cfg
	return b
}

func (b *checkoutSessionBuilder) WithCheckoutInvoiceID(invoiceID *string) *checkoutSessionBuilder {
	if b == nil || b.session == nil {
		return b
	}
	b.session.CheckoutInvoiceID = invoiceID
	return b
}

func (b *checkoutSessionBuilder) WithCheckoutParams(params *CheckoutParams) *checkoutSessionBuilder {
	if b == nil || b.session == nil || params == nil {
		return b
	}
	
	b.session.PaymentProvider = params.PaymentProvider
	b.session.PaymentProviderConfig = ToJSONBCheckoutPaymentProviderConfig(params.PaymentProviderConfig)
	b.session.IdempotencyKey = params.IdempotencyKey
	b.session.SuccessURL = params.SuccessURL
	b.session.FailureURL = params.FailureURL
	b.session.CancelURL = params.CancelURL
	b.session.Metadata = params.Metadata
	b.session.ExpiresAt = time.Now().UTC().Add(params.PaymentProvider.SessionExpiry())
	return b
}

// Build returns the session, or nil if the builder is nil.
func (b *checkoutSessionBuilder) Build() *CheckoutSession {
	if b == nil {
		return nil
	}
	return b.session
}
