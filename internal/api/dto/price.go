package dto

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type CreatePriceRequest struct {
	Amount             int                   `json:"amount" validate:"required"`
	Currency           string                `json:"currency" validate:"required,len=3"`
	PlanID             string                `json:"plan_id" validate:"required"`
	Type               types.PriceType       `json:"type" validate:"required"`
	BillingPeriod      types.BillingPeriod   `json:"billing_period" validate:"required"`
	BillingPeriodCount int                   `json:"billing_period_count" validate:"required,min=1"`
	BillingModel       types.BillingModel    `json:"billing_model" validate:"required"`
	BillingCadence     types.BillingCadence  `json:"billing_cadence" validate:"required"`
	MeterID            string                `json:"meter_id"`
	FilterValues       price.JSONBFilters    `json:"filter_values"`
	LookupKey          string                `json:"lookup_key"`
	Description        string                `json:"description"`
	Metadata           map[string]string     `json:"metadata"`
	TierMode           *types.BillingTier    `json:"tiers_mode"`
	Tiers              []price.PriceTier     `json:"tiers"`
	Transform          *price.JSONBTransform `json:"transform"`
}

// TODO : add all price validations
func (r *CreatePriceRequest) Validate() error {

	// Base validations
	if r.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}

	// Billing model validations
	err := validator.New().Struct(r)
	if err != nil {
		return err
	}

	switch r.BillingModel {
	case types.BILLING_MODEL_TIERED:
		if len(r.Tiers) == 0 {
			return fmt.Errorf("tiers are required when billing model is TIERED")
		}

	case types.BILLING_MODEL_PACKAGE:
		if r.Transform == nil {
			return fmt.Errorf("transform is required when billing model is PACKAGE")
		}
	}

	switch r.Type {
	case types.PRICE_TYPE_USAGE:
		if r.MeterID == "" {
			return fmt.Errorf("meter_id is required when type is USAGE")
		}
	}

	switch r.BillingCadence {
	case types.BILLING_CADENCE_RECURRING:
		if r.BillingPeriod == "" {
			return fmt.Errorf("billing_period is required when billing_cadence is RECURRING")
		}
	}
	return nil
}

func (r *CreatePriceRequest) ToPrice(ctx context.Context) *price.Price {
	price := &price.Price{
		ID:                 uuid.New().String(),
		Amount:             r.Amount,
		Currency:           r.Currency,
		PlanID:             r.PlanID,
		Type:               r.Type,
		BillingPeriod:      r.BillingPeriod,
		BillingPeriodCount: r.BillingPeriodCount,
		BillingModel:       r.BillingModel,
		BillingCadence:     r.BillingCadence,
		MeterID:            r.MeterID,
		FilterValues:       r.FilterValues,
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
	price.DisplayAmount = price.GetDisplayAmount()
	return price
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
