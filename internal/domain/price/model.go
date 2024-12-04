package price

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/flexprice/flexprice/internal/types"
)

// JSONB types for complex fields
type JSONBTiers []PriceTier
type JSONBTransform PriceTransform
type JSONBMetadata map[string]string

// Price model with JSONB tags
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
	BillingPeriodCount int `db:"billing_period_count" json:"billing_period_count"`

	// BillingModel is the billing model for the price ex FLAT_FEE, PER_UNIT, TIERED
	BillingModel types.BillingModel `db:"billing_model" json:"billing_model"`

	// BillingCadence is the billing cadence for the price ex RECURRING, ONETIME
	BillingCadence types.BillingCadence `db:"billing_cadence" json:"billing_cadence"`

	// BillingCountryCode 3 digit ISO country code in uppercase ex USA, FRA, DEU
	BillingCountryCode string `db:"billing_country_code" json:"billing_country_code"`

	// Tiered pricing fields
	TierMode *types.BillingTier `db:"tier_mode" json:"tier_mode"`
	Tiers    JSONBTiers         `db:"tiers,jsonb" json:"tiers"` // JSONB field

	// Quantity transformation
	Transform *JSONBTransform `db:"transform,jsonb" json:"transform"` // JSONB field

	// LookupKey used for looking up the price in the database
	LookupKey string `db:"lookup_key" json:"lookup_key"`

	// Description of the price
	Description string `db:"description" json:"description"`

	// Metadata is a jsonb field for additional information
	Metadata JSONBMetadata `db:"metadata,jsonb" json:"metadata"` // JSONB field

	types.BaseModel
}

// Scanner/Valuer implementations for JSONBTiers
func (j *JSONBTiers) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("invalid type for jsonb tiers")
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONBTiers) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scanner/Valuer implementations for JSONBTransform
func (j *JSONBTransform) Scan(value interface{}) error {
	if value == nil {
		*j = JSONBTransform{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("invalid type for jsonb transform")
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONBTransform) Value() (driver.Value, error) {
	if j == (JSONBTransform{}) {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scanner/Valuer implementations for JSONBMetadata
func (j *JSONBMetadata) Scan(value interface{}) error {
	if value == nil {
		*j = make(JSONBMetadata)
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("invalid type for jsonb metadata")
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONBMetadata) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

type PriceTransform struct {
	DivideBy int    `json:"divide_by"` // Divide quantity by this number
	Round    string `json:"round"`     // up, down, or nearest
}

type PriceTier struct {
	UpTo       *int `json:"up_to"`       // null means infinity
	UnitAmount int  `json:"unit_amount"` // Amount per unit in cents
	FlatAmount *int `json:"flat_amount"` // Optional flat fee for this tier
}
