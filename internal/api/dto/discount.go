package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// CreateDiscountRequest represents the request to create a new discount
type CreateDiscountRequest struct {
	CouponID               string  `json:"coupon_id" validate:"required"`
	SubscriptionID         *string `json:"subscription_id,omitempty"`
	SubscriptionLineItemID *string `json:"subscription_line_item_id,omitempty"`
}

// DiscountResponse represents the response for discount data
type DiscountResponse struct {
	ID                     string       `json:"id"`
	CouponID               string       `json:"coupon_id"`
	SubscriptionID         *string      `json:"subscription_id,omitempty"`
	SubscriptionLineItemID *string      `json:"subscription_line_item_id,omitempty"`
	TenantID               string       `json:"tenant_id"`
	Status                 types.Status `json:"status"`
	CreatedAt              time.Time    `json:"created_at"`
	UpdatedAt              time.Time    `json:"updated_at"`
	CreatedBy              string       `json:"created_by"`
	UpdatedBy              string       `json:"updated_by"`
	EnvironmentID          string       `json:"environment_id"`
}

// ListDiscountsRequest represents the request to list discounts
type ListDiscountsRequest struct {
	CouponID               *string                  `json:"coupon_id,omitempty"`
	SubscriptionID         *string                  `json:"subscription_id,omitempty"`
	SubscriptionLineItemID *string                  `json:"subscription_line_item_id,omitempty"`
	Pagination             types.PaginationResponse `json:"pagination"`
}

// ListDiscountsResponse represents the response for listing discounts
type ListDiscountsResponse struct {
	Discounts  []*DiscountResponse      `json:"discounts"`
	Pagination types.PaginationResponse `json:"pagination"`
	TotalCount int                      `json:"total_count"`
}
