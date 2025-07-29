package coupon

import (
	"context"
)

// Repository defines the interface for coupon data access
type Repository interface {
	Create(ctx context.Context, coupon *Coupon) error
	Get(ctx context.Context, id string) (*Coupon, error)
	GetByTenant(ctx context.Context, tenantID string, environmentID string) ([]*Coupon, error)
	GetActive(ctx context.Context, tenantID string, environmentID string) ([]*Coupon, error)
	Update(ctx context.Context, coupon *Coupon) error
	Delete(ctx context.Context, id string) error
	IncrementRedemptions(ctx context.Context, id string) error
	GetByStatus(ctx context.Context, tenantID string, environmentID string) ([]*Coupon, error)
	GetValidForCustomer(ctx context.Context, tenantID string, environmentID string, customerID string) ([]*Coupon, error)
	GetValidForSubscription(ctx context.Context, tenantID string, environmentID string, subscriptionID string) ([]*Coupon, error)
}
