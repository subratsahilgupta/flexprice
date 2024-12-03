package dto

import (
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/types"
)

type CreatePriceRequest struct {
	Amount             int                   `json:"amount" validate:"required"`
	Currency           string                `json:"currency" validate:"required,len=3"`
	ExternalID         string                `json:"external_id"`
	ExternalSource     string                `json:"external_source"`
	BillingPeriod      types.BillingPeriod   `json:"billing_period" validate:"required"`
	BillingPeriodCount int                   `json:"billing_period_count" validate:"required,min=1"`
	BillingModel       types.BillingModel    `json:"billing_model" validate:"required"`
	BillingCadence     types.BillingCadence  `json:"billing_cadence" validate:"required"`
	BillingCountryCode string                `json:"billing_country_code"`
	LookupKey          string                `json:"lookup_key" validate:"required"`
	Description        string                `json:"description"`
	Metadata           map[string]string     `json:"metadata"`
	UsageType          *types.UsageType      `json:"usage_type"`
	TierMode           *types.BillingTier    `json:"tiers_mode"`
	Tiers              []price.PriceTier     `json:"tiers"`
	Transform          *price.PriceTransform `json:"transform"`
}

type UpdatePriceRequest struct {
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
}

type PriceResponse struct {
	*price.Price
}

type ListPricesResponse struct {
	Prices []PriceResponse `json:"prices"`
	Total  int             `json:"total"`
	Offset int             `json:"offset"`
	Limit  int             `json:"limit"`
}
