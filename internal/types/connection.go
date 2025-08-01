package types

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/samber/lo"
)

// ConnectionMetadataType represents the type of connection metadata
type ConnectionMetadataType string

const (
	ConnectionMetadataTypeStripe   ConnectionMetadataType = "stripe"
	ConnectionMetadataTypeRazorpay ConnectionMetadataType = "razorpay"
	ConnectionMetadataTypePayPal   ConnectionMetadataType = "paypal"
	ConnectionMetadataTypeGeneric  ConnectionMetadataType = "generic"
)

func (t ConnectionMetadataType) Validate() error {
	allowedTypes := []ConnectionMetadataType{
		ConnectionMetadataTypeStripe,
		ConnectionMetadataTypeRazorpay,
		ConnectionMetadataTypePayPal,
		ConnectionMetadataTypeGeneric,
	}
	if !lo.Contains(allowedTypes, t) {
		return ierr.NewError("invalid connection metadata type").
			WithHint("Connection metadata type must be one of: stripe, razorpay, paypal, generic").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// StripeConnectionMetadata represents Stripe-specific connection metadata
type StripeConnectionMetadata struct {
	PublishableKey string `json:"publishable_key"`
	SecretKey      string `json:"secret_key"`
	WebhookSecret  string `json:"webhook_secret"`
	AccountID      string `json:"account_id,omitempty"`
}

// Validate validates the Stripe connection metadata
func (s *StripeConnectionMetadata) Validate() error {
	if s.PublishableKey == "" {
		return ierr.NewError("publishable_key is required").
			WithHint("Stripe publishable key is required").
			Mark(ierr.ErrValidation)
	}
	if s.SecretKey == "" {
		return ierr.NewError("secret_key is required").
			WithHint("Stripe secret key is required").
			Mark(ierr.ErrValidation)
	}
	if s.WebhookSecret == "" {
		return ierr.NewError("webhook_secret is required").
			WithHint("Stripe webhook secret is required").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// RazorpayConnectionMetadata represents Razorpay-specific connection metadata
type RazorpayConnectionMetadata struct {
	KeyID         string `json:"key_id"`
	KeySecret     string `json:"key_secret"`
	WebhookSecret string `json:"webhook_secret"`
	AccountID     string `json:"account_id,omitempty"`
}

// Validate validates the Razorpay connection metadata
func (r *RazorpayConnectionMetadata) Validate() error {
	if r.KeyID == "" {
		return ierr.NewError("key_id is required").
			WithHint("Razorpay key ID is required").
			Mark(ierr.ErrValidation)
	}
	if r.KeySecret == "" {
		return ierr.NewError("key_secret is required").
			WithHint("Razorpay key secret is required").
			Mark(ierr.ErrValidation)
	}
	if r.WebhookSecret == "" {
		return ierr.NewError("webhook_secret is required").
			WithHint("Razorpay webhook secret is required").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// PayPalConnectionMetadata represents PayPal-specific connection metadata
type PayPalConnectionMetadata struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	WebhookID    string `json:"webhook_id"`
	AccountID    string `json:"account_id,omitempty"`
}

// Validate validates the PayPal connection metadata
func (p *PayPalConnectionMetadata) Validate() error {
	if p.ClientID == "" {
		return ierr.NewError("client_id is required").
			WithHint("PayPal client ID is required").
			Mark(ierr.ErrValidation)
	}
	if p.ClientSecret == "" {
		return ierr.NewError("client_secret is required").
			WithHint("PayPal client secret is required").
			Mark(ierr.ErrValidation)
	}
	if p.WebhookID == "" {
		return ierr.NewError("webhook_id is required").
			WithHint("PayPal webhook ID is required").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// GenericConnectionMetadata represents generic connection metadata
type GenericConnectionMetadata struct {
	Data map[string]interface{} `json:"data"`
}

// Validate validates the generic connection metadata
func (g *GenericConnectionMetadata) Validate() error {
	if g.Data == nil {
		return ierr.NewError("data is required").
			WithHint("Generic connection metadata data is required").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// ConnectionMetadata represents structured connection metadata
type ConnectionMetadata struct {
	Type     ConnectionMetadataType      `json:"type"`
	Stripe   *StripeConnectionMetadata   `json:"stripe,omitempty"`
	Razorpay *RazorpayConnectionMetadata `json:"razorpay,omitempty"`
	PayPal   *PayPalConnectionMetadata   `json:"paypal,omitempty"`
	Generic  *GenericConnectionMetadata  `json:"generic,omitempty"`
}

// Validate validates the connection metadata
func (c *ConnectionMetadata) Validate() error {
	if err := c.Type.Validate(); err != nil {
		return err
	}

	switch c.Type {
	case ConnectionMetadataTypeStripe:
		if c.Stripe == nil {
			return ierr.NewError("stripe metadata is required").
				WithHint("Stripe metadata is required for stripe type").
				Mark(ierr.ErrValidation)
		}
		return c.Stripe.Validate()
	case ConnectionMetadataTypeRazorpay:
		if c.Razorpay == nil {
			return ierr.NewError("razorpay metadata is required").
				WithHint("Razorpay metadata is required for razorpay type").
				Mark(ierr.ErrValidation)
		}
		return c.Razorpay.Validate()
	case ConnectionMetadataTypePayPal:
		if c.PayPal == nil {
			return ierr.NewError("paypal metadata is required").
				WithHint("PayPal metadata is required for paypal type").
				Mark(ierr.ErrValidation)
		}
		return c.PayPal.Validate()
	case ConnectionMetadataTypeGeneric:
		if c.Generic == nil {
			return ierr.NewError("generic metadata is required").
				WithHint("Generic metadata is required for generic type").
				Mark(ierr.ErrValidation)
		}
		return c.Generic.Validate()
	default:
		return ierr.NewError("invalid metadata type").
			WithHint("Invalid metadata type").
			Mark(ierr.ErrValidation)
	}
}

// ConnectionFilter represents filters for connection queries
type ConnectionFilter struct {
	*QueryFilter
	*TimeRangeFilter
	// filters allows complex filtering based on multiple fields

	Filters       []*FilterCondition `json:"filters,omitempty" form:"filters" validate:"omitempty"`
	Sort          []*SortCondition   `json:"sort,omitempty" form:"sort" validate:"omitempty"`
	ConnectionIDs []string           `json:"connection_ids,omitempty" form:"connection_ids" validate:"omitempty"`
	ProviderType  SecretProvider     `json:"provider_type,omitempty" form:"provider_type" validate:"omitempty"`
}

// NewConnectionFilter creates a new ConnectionFilter with default values
func NewConnectionFilter() *ConnectionFilter {
	return &ConnectionFilter{
		QueryFilter: NewDefaultQueryFilter(),
	}
}

// NewNoLimitConnectionFilter creates a new ConnectionFilter with no pagination limits
func NewNoLimitConnectionFilter() *ConnectionFilter {
	return &ConnectionFilter{
		QueryFilter: NewNoLimitQueryFilter(),
	}
}

// Validate validates the connection filter
func (f ConnectionFilter) Validate() error {
	if f.QueryFilter != nil {
		if err := f.QueryFilter.Validate(); err != nil {
			return err
		}
	}

	if f.TimeRangeFilter != nil {
		if err := f.TimeRangeFilter.Validate(); err != nil {
			return err
		}
	}

	if f.ProviderType != "" && !f.ProviderType.IsValid() {
		return ierr.NewError("invalid provider type").
			WithHint("Please provide a valid provider type").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// GetLimit implements BaseFilter interface
func (f *ConnectionFilter) GetLimit() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetLimit()
	}
	return f.QueryFilter.GetLimit()
}

// GetOffset implements BaseFilter interface
func (f *ConnectionFilter) GetOffset() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOffset()
	}
	return f.QueryFilter.GetOffset()
}

// GetSort implements BaseFilter interface
func (f *ConnectionFilter) GetSort() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetSort()
	}
	return f.QueryFilter.GetSort()
}

// GetOrder implements BaseFilter interface
func (f *ConnectionFilter) GetOrder() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOrder()
	}
	return f.QueryFilter.GetOrder()
}

// GetStatus implements BaseFilter interface
func (f *ConnectionFilter) GetStatus() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetStatus()
	}
	return f.QueryFilter.GetStatus()
}

// IsUnlimited implements BaseFilter interface
func (f *ConnectionFilter) IsUnlimited() bool {
	if f.QueryFilter == nil {
		return false
	}
	return f.QueryFilter.IsUnlimited()
}
