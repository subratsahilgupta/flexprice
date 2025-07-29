package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// CreateRedemptionRequest represents the request to create a new redemption
type CreateRedemptionRequest struct {
	CouponID           string                 `json:"coupon_id" validate:"required"`
	DiscountID         string                 `json:"discount_id" validate:"required"`
	InvoiceID          string                 `json:"invoice_id" validate:"required"`
	InvoiceLineItemID  *string                `json:"invoice_line_item_id,omitempty"`
	OriginalPrice      decimal.Decimal        `json:"original_price" validate:"required"`
	FinalPrice         decimal.Decimal        `json:"final_price" validate:"required"`
	DiscountedAmount   decimal.Decimal        `json:"discounted_amount" validate:"required"`
	DiscountType       types.DiscountType     `json:"discount_type" validate:"required"`
	DiscountPercentage *decimal.Decimal       `json:"discount_percentage,omitempty"`
	Currency           string                 `json:"currency" validate:"required"`
	CouponSnapshot     map[string]interface{} `json:"coupon_snapshot,omitempty"`
}

// RedemptionResponse represents the response for redemption data
type RedemptionResponse struct {
	ID                 string                 `json:"id"`
	CouponID           string                 `json:"coupon_id"`
	DiscountID         string                 `json:"discount_id"`
	InvoiceID          string                 `json:"invoice_id"`
	InvoiceLineItemID  *string                `json:"invoice_line_item_id,omitempty"`
	RedeemedAt         time.Time              `json:"redeemed_at"`
	OriginalPrice      decimal.Decimal        `json:"original_price"`
	FinalPrice         decimal.Decimal        `json:"final_price"`
	DiscountedAmount   decimal.Decimal        `json:"discounted_amount"`
	DiscountType       types.DiscountType     `json:"discount_type"`
	DiscountPercentage *decimal.Decimal       `json:"discount_percentage,omitempty"`
	Currency           string                 `json:"currency"`
	CouponSnapshot     map[string]interface{} `json:"coupon_snapshot,omitempty"`
	TenantID           string                 `json:"tenant_id"`
	Status             types.Status           `json:"status"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	CreatedBy          string                 `json:"created_by"`
	UpdatedBy          string                 `json:"updated_by"`
	EnvironmentID      string                 `json:"environment_id"`
}

// ListRedemptionsRequest represents the request to list redemptions
type ListRedemptionsRequest struct {
	CouponID          *string                  `json:"coupon_id,omitempty"`
	DiscountID        *string                  `json:"discount_id,omitempty"`
	InvoiceID         *string                  `json:"invoice_id,omitempty"`
	InvoiceLineItemID *string                  `json:"invoice_line_item_id,omitempty"`
	DiscountType      *types.DiscountType      `json:"discount_type,omitempty"`
	Pagination        types.PaginationResponse `json:"pagination"`
}

// ListRedemptionsResponse represents the response for listing redemptions
type ListRedemptionsResponse struct {
	Redemptions []*RedemptionResponse    `json:"redemptions"`
	Pagination  types.PaginationResponse `json:"pagination"`
	TotalCount  int                      `json:"total_count"`
}
