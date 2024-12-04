package dto

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
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
	LookupKey          string                `json:"lookup_key"`
	Description        string                `json:"description"`
	Metadata           map[string]string     `json:"metadata"`
	UsageType          *types.UsageType      `json:"usage_type"`
	TierMode           *types.BillingTier    `json:"tiers_mode"`
	Tiers              []price.PriceTier     `json:"tiers"`
	Transform          *price.JSONBTransform `json:"transform"`
}

func (r *CreatePriceRequest) Validate() error {
	return validator.New().Struct(r)
}

func (r *CreatePriceRequest) ToPrice(ctx context.Context) *price.Price {
	return &price.Price{
		ID:                 uuid.New().String(),
		Amount:             r.Amount,
		Currency:           r.Currency,
		ExternalID:         r.ExternalID,
		ExternalSource:     r.ExternalSource,
		BillingPeriod:      r.BillingPeriod,
		BillingPeriodCount: r.BillingPeriodCount,
		BillingModel:       r.BillingModel,
		BillingCadence:     r.BillingCadence,
		BillingCountryCode: r.BillingCountryCode,
		LookupKey:          r.LookupKey,
		Description:        r.Description,
		Metadata:           r.Metadata,
		TierMode:           r.TierMode,
		Tiers:              r.Tiers,
		Transform:          r.Transform,
		BaseModel: types.BaseModel{
			TenantID:  types.GetTenantID(ctx),
			Status:    types.StatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			CreatedBy: types.GetUserID(ctx),
			UpdatedBy: types.GetUserID(ctx),
		},
	}
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
