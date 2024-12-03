package price

import "github.com/flexprice/flexprice/internal/types"

type Price struct {
	// ID uuid identifier for the price
	ID string `db:"id" json:"id"`

	// Amount in cents ex 1200 for $12
	Amount int `db:"amount" json:"amount"`

	// Currency 3 digit ISO currency code in lowercase ex usd, eur, gbp
	Currency string `db:"currency" json:"currency"`

	// ExternalID is the identifier of the price in the external system
	ExternalID string `db:"external_id" json:"external_id"`

	// ExternalSource is the source of the price in the external system
	ExternalSource string `db:"external_source" json:"external_source"`

	// BillingPeriod is the billing period for the price ex month, year
	BillingPeriod types.BillingPeriod `db:"billing_period" json:"billing_period"`

	// BillingPeriodCount is the count of the billing period ex 1, 3, 6, 12
	// For example, if the billing period is month and the count is 3, then the billing period is 3 months
	BillingPeriodCount int `db:"billing_period_count" json:"billing_period_count"`

	// BillingModel is the billing model for the price ex FLAT_FEE, PER_UNIT, TIERED
	BillingModel types.BillingModel `db:"billing_model" json:"billing_model"`

	// BillingCadence is the billing cadence for the price ex RECURRING, ONETIME
	BillingCadence types.BillingCadence `db:"billing_cadence" json:"billing_cadence"`

	// BillingCountryCode 3 digit ISO country code in uppercase ex USA, FRA, DEU will be empty for global prices
	BillingCountryCode string `db:"billing_country_code" json:"billing_country_code"`

	// Tiered pricing fields
	TierMode *types.BillingTier `db:"tier_mode" json:"tier_mode"` // volume or slab(graduated)
	Tiers    []PriceTier         `db:"tiers" json:"tiers"`        // Array of pricing tiers

	// Quantity transformation
	Transform *PriceTransform `db:"transform" json:"transform"` // For package pricing

	// LookupKey used for looking up the price in the database
	LookupKey string `db:"lookup_key" json:"lookup_key"`

	// Description of the price
	Description string `db:"description" json:"description"`

	// Metadata is a jsonb field that can be used to store additional information about the price
	Metadata map[string]string `db:"metadata" json:"metadata"`

	types.BaseModel
}

type PriceTier struct {
	UpTo       *int `json:"up_to"`       // null means infinity
	UnitAmount int  `json:"unit_amount"` // Amount per unit in cents
	FlatAmount *int `json:"flat_amount"` // Optional flat fee for this tier
}

type PriceTransform struct {
	DivideBy int    `json:"divide_by"` // Divide quantity by this number
	Round    string `json:"round"`     // up, down, or nearest
}
