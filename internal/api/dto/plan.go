package dto

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type CreatePlanRequest struct {
	Name        string                   `json:"name" validate:"required"`
	Description string                   `json:"description"`
	Prices      []CreatePlanPriceRequest `json:"prices"`
}

type CreatePlanPriceRequest struct {
	*CreatePriceRequest
}

func (r *CreatePlanRequest) Validate() error {
	err := validator.New().Struct(r)
	if err != nil {
		return err
	}

	for _, price := range r.Prices {
		if err := price.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func (r *CreatePlanRequest) ToPlan(ctx context.Context) *plan.Plan {
	plan := &plan.Plan{
		ID:          uuid.New().String(),
		Name:        r.Name,
		Description: r.Description,
		BaseModel: types.BaseModel{
			TenantID:  types.GetTenantID(ctx),
			Status:    types.StatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			CreatedBy: types.GetUserID(ctx),
			UpdatedBy: types.GetUserID(ctx),
		},
	}
	return plan
}

type CreatePlanResponse struct {
	*plan.Plan
}

type PlanResponse struct {
	*plan.Plan
	Prices []PriceResponse `json:"prices"`
}

type UpdatePlanRequest struct {
	Name        string                   `json:"name" validate:"required"`
	Description string                   `json:"description"`
	Prices      []UpdatePlanPriceRequest `json:"prices"`
}

type UpdatePlanPriceRequest struct {
	PriceID string `json:"price_id" validate:"required"`
	*UpdatePriceRequest
}

type ListPlansResponse struct {
	Plans  []plan.Plan `json:"plans"`
	Total  int         `json:"total"`
	Offset int         `json:"offset"`
	Limit  int         `json:"limit"`
}
