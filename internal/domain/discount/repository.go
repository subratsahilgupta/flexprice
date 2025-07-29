package discount

import (
	"context"
)

// Repository defines the interface for discount data access
type Repository interface {
	Create(ctx context.Context, discount *Discount) error
	Get(ctx context.Context, id string) (*Discount, error)
	Update(ctx context.Context, discount *Discount) error
	Delete(ctx context.Context, id string) error
	GetBySubscription(ctx context.Context, subscriptionID string) ([]*Discount, error)
	GetBySubscriptionLineItem(ctx context.Context, subscriptionLineItemID string) ([]*Discount, error)
}
