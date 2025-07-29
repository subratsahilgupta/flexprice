package discount

import (
	"context"
)

// Repository defines the interface for discount data access
type Repository interface {
	Create(ctx context.Context, discount *Discount) error
	Get(ctx context.Context, id string) (*Discount, error)
	GetByCoupon(ctx context.Context, couponID string) ([]*Discount, error)
	GetBySubscription(ctx context.Context, subscriptionID string) ([]*Discount, error)
	GetBySubscriptionLineItem(ctx context.Context, subscriptionLineItemID string) ([]*Discount, error)
	Update(ctx context.Context, discount *Discount) error
	Delete(ctx context.Context, id string) error
	GetByTenant(ctx context.Context, tenantID string, environmentID string) ([]*Discount, error)
}
