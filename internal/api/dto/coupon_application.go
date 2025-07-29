package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// CreateCouponApplicationRequest represents the request to create a new coupon application
type CreateCouponApplicationRequest struct {
	CouponID            string                 `json:"coupon_id" validate:"required"`
	CouponAssociationID string                 `json:"coupon_association_id" validate:"required"`
	InvoiceID           string                 `json:"invoice_id" validate:"required"`
	InvoiceLineItemID   *string                `json:"invoice_line_item_id,omitempty"`
	OriginalPrice       decimal.Decimal        `json:"original_price" validate:"required"`
	FinalPrice          decimal.Decimal        `json:"final_price" validate:"required"`
	DiscountedAmount    decimal.Decimal        `json:"discounted_amount" validate:"required"`
	DiscountType        types.DiscountType     `json:"discount_type" validate:"required"`
	DiscountPercentage  *decimal.Decimal       `json:"discount_percentage,omitempty"`
	Currency            string                 `json:"currency" validate:"required"`
	CouponSnapshot      map[string]interface{} `json:"coupon_snapshot,omitempty"`
}

// CouponApplicationResponse represents the response for coupon application data
type CouponApplicationResponse struct {
	ID                  string                 `json:"id"`
	CouponID            string                 `json:"coupon_id"`
	CouponAssociationID string                 `json:"coupon_association_id"`
	InvoiceID           string                 `json:"invoice_id"`
	InvoiceLineItemID   *string                `json:"invoice_line_item_id,omitempty"`
	AppliedAt           time.Time              `json:"applied_at"`
	OriginalPrice       decimal.Decimal        `json:"original_price"`
	FinalPrice          decimal.Decimal        `json:"final_price"`
	DiscountedAmount    decimal.Decimal        `json:"discounted_amount"`
	DiscountType        types.DiscountType     `json:"discount_type"`
	DiscountPercentage  *decimal.Decimal       `json:"discount_percentage,omitempty"`
	Currency            string                 `json:"currency"`
	CouponSnapshot      map[string]interface{} `json:"coupon_snapshot,omitempty"`
	TenantID            string                 `json:"tenant_id"`
	Status              types.Status           `json:"status"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
	CreatedBy           string                 `json:"created_by"`
	UpdatedBy           string                 `json:"updated_by"`
	EnvironmentID       string                 `json:"environment_id"`
}

// ListCouponApplicationsRequest represents the request to list coupon applications
type ListCouponApplicationsRequest struct {
	CouponID            *string                  `json:"coupon_id,omitempty"`
	CouponAssociationID *string                  `json:"coupon_association_id,omitempty"`
	InvoiceID           *string                  `json:"invoice_id,omitempty"`
	InvoiceLineItemID   *string                  `json:"invoice_line_item_id,omitempty"`
	DiscountType        *types.DiscountType      `json:"discount_type,omitempty"`
	Pagination          types.PaginationResponse `json:"pagination"`
}

// ListCouponApplicationsResponse represents the response for listing coupon applications
type ListCouponApplicationsResponse struct {
	CouponApplications []*CouponApplicationResponse `json:"coupon_applications"`
	Pagination         types.PaginationResponse     `json:"pagination"`
	TotalCount         int                          `json:"total_count"`
}
