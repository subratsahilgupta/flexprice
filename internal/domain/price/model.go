package price

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/schema"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// JSONB types for complex fields
// JSONBTiers are the tiers for the price when BillingModel is TIERED
type JSONBTiers []PriceTier

// JSONBTransformQuantity is the quantity transformation in case of PACKAGE billing model
type JSONBTransformQuantity TransformQuantity

// JSONBMetadata is a jsonb field for additional information
type JSONBMetadata map[string]string

// JSONBFilters are the filter values for the price in case of usage based pricing
type JSONBFilters map[string][]string

// Price model with JSONB tags
type Price struct {
	// ID uuid identifier for the price
	ID string `db:"id" json:"id"`

	// Amount stored in main currency units (e.g., dollars, not cents)
	// For USD: 12.50 means $12.50
	Amount decimal.Decimal `db:"amount" json:"amount"`

	// DisplayAmount is the formatted amount with currency symbol
	// For USD: $12.50
	DisplayAmount string `db:"display_amount" json:"display_amount"`

	// Currency 3 digit ISO currency code in lowercase ex usd, eur, gbp
	Currency string `db:"currency" json:"currency"`

	// PriceUnitType is the type of the price unit- Fiat, Custom, Crypto
	PriceUnitType types.PriceUnitType `db:"price_unit_type" json:"price_unit_type"`

	// PriceUnitID is the id of the price unit
	PriceUnitID string `db:"price_unit_id" json:"price_unit_id,omitempty"`

	// PriceUnitAmount is the amount stored in price unit
	// For BTC: 0.00000001 means 0.00000001 BTC
	PriceUnitAmount decimal.Decimal `db:"price_unit_amount" json:"price_unit_amount,omitempty"`

	// DisplayPriceUnitAmount is the formatted amount with price unit symbol
	// For BTC: 0.00000001 BTC
	DisplayPriceUnitAmount string `db:"display_price_unit_amount" json:"display_price_unit_amount,omitempty"`

	// PriceUnit 3 digit ISO currency code in lowercase ex btc
	// For BTC: btc
	PriceUnit string `db:"price_unit" json:"price_unit,omitempty"`

	// ConversionRate is the rate of the price unit to the base currency
	// For BTC: 1 BTC = 100000000 USD
	ConversionRate decimal.Decimal `db:"conversion_rate" json:"conversion_rate,omitempty"`

	// PlanID is the id of the plan for plan based pricing
	PlanID string `db:"plan_id" json:"plan_id"`

	Type types.PriceType `db:"type" json:"type"`

	BillingPeriod types.BillingPeriod `db:"billing_period" json:"billing_period"`

	// BillingPeriodCount is the count of the billing period ex 1, 3, 6, 12
	BillingPeriodCount int `db:"billing_period_count" json:"billing_period_count"`

	BillingModel types.BillingModel `db:"billing_model" json:"billing_model"`

	BillingCadence types.BillingCadence `db:"billing_cadence" json:"billing_cadence"`

	InvoiceCadence types.InvoiceCadence `db:"invoice_cadence" json:"invoice_cadence"`

	// TrialPeriod is the number of days for the trial period
	// Note: This is only applicable for recurring prices (BILLING_CADENCE_RECURRING)
	TrialPeriod int `db:"trial_period" json:"trial_period"`

	TierMode types.BillingTier `db:"tier_mode" json:"tier_mode"`

	Tiers JSONBTiers `db:"tiers,jsonb" json:"tiers"`

	// PriceUnitTiers are the tiers for the price unit
	PriceUnitTiers JSONBTiers `db:"price_unit_tiers,jsonb" json:"price_unit_tiers"`

	// MeterID is the id of the meter for usage based pricing
	MeterID string `db:"meter_id" json:"meter_id"`

	// LookupKey used for looking up the price in the database
	LookupKey string `db:"lookup_key" json:"lookup_key"`

	// Description of the price
	Description string `db:"description" json:"description"`

	TransformQuantity JSONBTransformQuantity `db:"transform_quantity,jsonb" json:"transform_quantity"`

	Metadata JSONBMetadata `db:"metadata,jsonb" json:"metadata"`

	// EnvironmentID is the environment identifier for the price
	EnvironmentID string `db:"environment_id" json:"environment_id"`

	// Price override fields
	// Scope indicates if this is a plan-level or subscription-level price
	Scope types.PriceScope `db:"scope" json:"scope"`

	// ParentPriceID references the original price (only set when scope is SUBSCRIPTION)
	ParentPriceID string `db:"parent_price_id" json:"parent_price_id,omitempty"`

	// SubscriptionID references the subscription (only set when scope is SUBSCRIPTION)
	SubscriptionID string `db:"subscription_id" json:"subscription_id,omitempty"`

	types.BaseModel
}

// IsUsage returns true if the price is a usage based price
func (p *Price) IsUsage() bool {
	return p.Type == types.PRICE_TYPE_USAGE && p.MeterID != ""
}

// IsSubscriptionScoped returns true if the price is subscription-scoped
func (p *Price) IsSubscriptionScoped() bool {
	return p.Scope == types.PRICE_SCOPE_SUBSCRIPTION
}

// IsPlanScoped returns true if the price is plan-scoped
func (p *Price) IsPlanScoped() bool {
	return p.Scope == types.PRICE_SCOPE_PLAN
}

// GetCurrencySymbol returns the currency symbol for the price
func (p *Price) GetCurrencySymbol() string {
	return types.GetCurrencySymbol(p.Currency)
}

// ValidateAmount checks if amount is within valid range for price definition
func (p *Price) ValidateAmount() error {
	if p.Amount.LessThan(decimal.Zero) {
		return ierr.NewError("amount must be greater than 0").
			WithHint("Please provide a positive amount value").
			WithReportableDetails(map[string]interface{}{
				"amount": p.Amount.String(),
			}).
			Mark(ierr.ErrValidation)
	}
	return nil
}

// FormatAmountToString formats the amount to string
func (p *Price) FormatAmountToString() string {
	return p.Amount.String()
}

// FormatAmountToStringWithPrecision formats the amount to string
// It rounds off the amount according to currency precision
func (p *Price) FormatAmountToStringWithPrecision() string {
	config := types.GetCurrencyConfig(p.Currency)
	return p.Amount.Round(config.Precision).String()
}

// FormatAmountToFloat64 formats the amount to float64
func (p *Price) FormatAmountToFloat64() float64 {
	return p.Amount.InexactFloat64()
}

// FormatAmountToFloat64WithPrecision formats the amount to float64
// It rounds off the amount according to currency precision
func (p *Price) FormatAmountToFloat64WithPrecision() float64 {
	config := types.GetCurrencyConfig(p.Currency)
	return p.Amount.Round(config.Precision).InexactFloat64()
}

// GetDisplayAmount returns the amount in the currency ex $12.00
func (p *Price) GetDisplayAmount() string {
	amount := p.FormatAmountToString()
	return fmt.Sprintf("%s%s", p.GetCurrencySymbol(), amount)
}

// CalculateAmount performs calculation
func (p *Price) CalculateAmount(quantity decimal.Decimal) decimal.Decimal {
	// Calculate with full precision
	result := p.Amount.Mul(quantity)
	return result
}

// CalculateTierAmount performs calculation for tier price with flat and fixed ampunt
func (pt *PriceTier) CalculateTierAmount(quantity decimal.Decimal, currency string) decimal.Decimal {
	tierCost := pt.UnitAmount.Mul(quantity)
	if pt.FlatAmount != nil {
		tierCost = tierCost.Add(*pt.FlatAmount)
	}
	return tierCost
}

func (pt *PriceTier) GetPerUnitCost() decimal.Decimal {
	return pt.UnitAmount
}

// GetDisplayAmount returns the amount in the currency ex $12.00
func GetDisplayAmountWithPrecision(amount decimal.Decimal, currency string) string {
	val := FormatAmountToStringWithPrecision(amount, currency)
	config := types.GetCurrencyConfig(currency)
	return fmt.Sprintf("%s%s", config.Symbol, val)
}

// FormatAmountToStringWithPrecision formats the amount to string
// It rounds off the amount according to currency precision
func FormatAmountToStringWithPrecision(amount decimal.Decimal, currency string) string {
	config := types.GetCurrencyConfig(currency)
	return amount.Round(config.Precision).String()
}

// FormatAmountToFloat64WithPrecision formats the amount to float64
// It rounds off the amount according to currency precision
func FormatAmountToFloat64WithPrecision(amount decimal.Decimal, currency string) float64 {
	return amount.Round(types.GetCurrencyPrecision(currency)).InexactFloat64()
}

// PriceTransform is the quantity transformation in case of PACKAGE billing model
// NOTE: We need to apply this to the quantity before calculating the effective price
type TransformQuantity struct {
	DivideBy int    `json:"divide_by,omitempty"` // Divide quantity by this number
	Round    string `json:"round,omitempty"`     // up or down
}

type PriceTier struct {
	// Upto is the quantity up to which this tier applies. It is null for the last tier
	UpTo *uint64 `json:"up_to"`
	// UnitAmount is the amount per unit for the given tier
	UnitAmount decimal.Decimal `json:"unit_amount"`
	// FlatAmount is the flat amount for the given tier and it is applied
	// on top of the unit amount*quantity. It solves cases in banking like 2.7% + 5c
	FlatAmount *decimal.Decimal `json:"flat_amount,omitempty"`
}

// TODO : comeup with a better way to handle jsonb fields

// Scanner/Valuer implementations for JSONBTiers
func (j *JSONBTiers) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return ierr.NewError("invalid type for jsonb tiers").
			WithHint("Invalid type for JSONB tiers").
			Mark(ierr.ErrValidation)
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONBTiers) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// GetTierUpTo returns the up_to value for the tier and treats null case as MaxUint64.
// NOTE: Only to be used for sorting of tiers to avoid any unexpected behaviour
func (t PriceTier) GetTierUpTo() uint64 {
	if t.UpTo != nil {
		return *t.UpTo
	}
	return math.MaxUint64
}

// Scanner/Valuer implementations for JSONBTransform
func (j *JSONBTransformQuantity) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return ierr.NewError("invalid type for jsonb transform").
			WithHint("Invalid type for JSONB transform").
			Mark(ierr.ErrValidation)
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONBTransformQuantity) Value() (driver.Value, error) {
	if j == (JSONBTransformQuantity{}) {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scanner/Valuer implementations for JSONBMetadata
func (j *JSONBMetadata) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return ierr.NewError("invalid type for jsonb metadata").
			WithHint("Invalid type for JSONB metadata").
			Mark(ierr.ErrValidation)
	}
	return json.Unmarshal(bytes, &j)
}

func (j JSONBMetadata) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONBFilters) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return ierr.NewError("invalid type for jsonb filters").
			WithHint("Invalid type for JSONB filters").
			Mark(ierr.ErrValidation)
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONBFilters) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// FromEnt converts an Ent Price to a domain Price
func FromEnt(e *ent.Price) *Price {
	if e == nil {
		return nil
	}

	// Convert tiers from ent model to price tiers
	var tiers JSONBTiers
	if len(e.Tiers) > 0 {
		tiers = make(JSONBTiers, len(e.Tiers))
		for i, tier := range e.Tiers {
			tiers[i] = PriceTier{
				UpTo:       tier.UpTo,
				UnitAmount: tier.UnitAmount,
			}
			if tier.FlatAmount != nil {
				flatAmount := tier.FlatAmount
				tiers[i].FlatAmount = flatAmount
			}
		}
	}

	// Convert price unit tiers from ent model to price tiers
	var priceUnitTiers JSONBTiers
	if len(e.PriceUnitTiers) > 0 {
		priceUnitTiers = make(JSONBTiers, len(e.PriceUnitTiers))
		for i, tier := range e.PriceUnitTiers {
			priceUnitTiers[i] = PriceTier{
				UpTo:       tier.UpTo,
				UnitAmount: tier.UnitAmount,
			}
			if tier.FlatAmount != nil {
				flatAmount := tier.FlatAmount
				priceUnitTiers[i].FlatAmount = flatAmount
			}
		}
	}

	return &Price{
		ID:                     e.ID,
		Amount:                 decimal.NewFromFloat(e.Amount),
		Currency:               e.Currency,
		DisplayAmount:          e.DisplayAmount,
		PriceUnitType:          types.PriceUnitType(e.PriceUnitType),
		PlanID:                 e.PlanID,
		Type:                   types.PriceType(e.Type),
		BillingPeriod:          types.BillingPeriod(e.BillingPeriod),
		BillingPeriodCount:     e.BillingPeriodCount,
		BillingModel:           types.BillingModel(e.BillingModel),
		BillingCadence:         types.BillingCadence(e.BillingCadence),
		InvoiceCadence:         types.InvoiceCadence(e.InvoiceCadence),
		TrialPeriod:            e.TrialPeriod,
		TierMode:               types.BillingTier(lo.FromPtr(e.TierMode)),
		Tiers:                  tiers,
		PriceUnitTiers:         priceUnitTiers,
		MeterID:                lo.FromPtr(e.MeterID),
		LookupKey:              e.LookupKey,
		Description:            e.Description,
		TransformQuantity:      JSONBTransformQuantity(e.TransformQuantity),
		Metadata:               JSONBMetadata(e.Metadata),
		EnvironmentID:          e.EnvironmentID,
		PriceUnitID:            e.PriceUnitID,
		PriceUnit:              e.PriceUnit,
		PriceUnitAmount:        decimal.NewFromFloat(e.PriceUnitAmount),
		DisplayPriceUnitAmount: e.DisplayPriceUnitAmount,
		ConversionRate:         decimal.NewFromFloat(e.ConversionRate),
		Scope:                  types.PriceScope(e.Scope),
		ParentPriceID:          lo.FromPtr(e.ParentPriceID),
		SubscriptionID:         lo.FromPtr(e.SubscriptionID),
		BaseModel: types.BaseModel{
			TenantID:  e.TenantID,
			Status:    types.Status(e.Status),
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
			CreatedBy: e.CreatedBy,
			UpdatedBy: e.UpdatedBy,
		},
	}
}

// FromEntList converts a list of Ent Prices to domain Prices
func FromEntList(list []*ent.Price) []*Price {
	if list == nil {
		return nil
	}
	prices := make([]*Price, len(list))
	for i, item := range list {
		prices[i] = FromEnt(item)
	}
	return prices
}

// ToEntTiers converts domain tiers to ent tiers
func (p *Price) ToEntTiers() []schema.PriceTier {
	if len(p.Tiers) == 0 {
		return nil
	}

	tiers := make([]schema.PriceTier, len(p.Tiers))
	for i, tier := range p.Tiers {
		tiers[i] = schema.PriceTier{
			UpTo:       tier.UpTo,
			UnitAmount: tier.UnitAmount,
			FlatAmount: tier.FlatAmount,
		}
	}
	return tiers
}

// ToPriceUnitTiers converts domain price unit tiers to ent tiers
func (p *Price) ToPriceUnitTiers() []schema.PriceTier {
	if len(p.PriceUnitTiers) == 0 {
		return nil
	}

	tiers := make([]schema.PriceTier, len(p.PriceUnitTiers))
	for i, tier := range p.PriceUnitTiers {
		tiers[i] = schema.PriceTier{
			UpTo:       tier.UpTo,
			UnitAmount: tier.UnitAmount,
			FlatAmount: tier.FlatAmount,
		}
	}
	return tiers
}

// ValidateTrialPeriod checks if trial period is valid
func (p *Price) ValidateTrialPeriod() error {
	// Trial period should be non-negative
	if p.TrialPeriod < 0 {
		return ierr.NewError("trial period must be non-negative").
			WithHint("Trial period must be non-negative").
			Mark(ierr.ErrValidation)
	}

	// Trial period should only be set for recurring fixed prices
	if p.TrialPeriod > 0 &&
		p.BillingCadence != types.BILLING_CADENCE_RECURRING &&
		p.Type != types.PRICE_TYPE_FIXED {
		return ierr.NewError("trial period can only be set for recurring fixed prices").
			WithHint("Trial period can only be set for recurring fixed prices").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// ValidateInvoiceCadence checks if invoice cadence is valid
func (p *Price) ValidateInvoiceCadence() error {
	return p.InvoiceCadence.Validate()
}

// ValidateOverrideFields validates price override fields
func (p *Price) ValidateOverrideFields() error {
	// Validate scope
	if err := p.Scope.Validate(); err != nil {
		return err
	}

	// For subscription-scoped prices
	if p.IsSubscriptionScoped() {
		// Parent price ID is required
		if p.ParentPriceID == "" {
			return ierr.NewError("parent_price_id is required for subscription-scoped prices").
				WithHint("Subscription-scoped prices must reference a parent price").
				Mark(ierr.ErrValidation)
		}

		// Subscription ID is required
		if p.SubscriptionID == "" {
			return ierr.NewError("subscription_id is required for subscription-scoped prices").
				WithHint("Subscription-scoped prices must reference a subscription").
				Mark(ierr.ErrValidation)
		}
	} else {
		// For plan-scoped prices, parent_price_id and subscription_id should be empty
		if p.ParentPriceID != "" {
			return ierr.NewError("parent_price_id should be empty for plan-scoped prices").
				WithHint("Plan-scoped prices should not have a parent price").
				Mark(ierr.ErrValidation)
		}

		if p.SubscriptionID != "" {
			return ierr.NewError("subscription_id should be empty for plan-scoped prices").
				WithHint("Plan-scoped prices should not reference a subscription").
				Mark(ierr.ErrValidation)
		}
	}

	return nil
}

// Validate performs all validations on the price
func (p *Price) Validate() error {
	if err := p.ValidateAmount(); err != nil {
		return err
	}

	if err := p.ValidateTrialPeriod(); err != nil {
		return err
	}

	if err := p.ValidateInvoiceCadence(); err != nil {
		return err
	}

	if err := p.ValidateOverrideFields(); err != nil {
		return err
	}

	return nil
}
