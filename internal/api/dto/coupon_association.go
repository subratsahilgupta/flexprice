package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// CreateCouponAssociationRequest represents the request to create a new coupon association
type CreateCouponAssociationRequest struct {
	CouponID               string  `json:"coupon_id" validate:"required"`
	SubscriptionID         *string `json:"subscription_id,omitempty"`
	SubscriptionLineItemID *string `json:"subscription_line_item_id,omitempty"`
}

// CouponAssociationResponse represents the response for coupon association data
type CouponAssociationResponse struct {
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

// ListCouponAssociationsRequest represents the request to list coupon associations
type ListCouponAssociationsRequest struct {
	CouponID               *string                  `json:"coupon_id,omitempty"`
	SubscriptionID         *string                  `json:"subscription_id,omitempty"`
	SubscriptionLineItemID *string                  `json:"subscription_line_item_id,omitempty"`
	Pagination             types.PaginationResponse `json:"pagination"`
}

// ListCouponAssociationsResponse represents the response for listing coupon associations
type ListCouponAssociationsResponse struct {
	CouponAssociations []*CouponAssociationResponse `json:"coupon_associations"`
	Pagination         types.PaginationResponse     `json:"pagination"`
	TotalCount         int                          `json:"total_count"`
}
