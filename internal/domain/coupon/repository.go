package coupon

import (
	"context"
)

// Repository defines the interface for coupon data access
type Repository interface {
	Create(ctx context.Context, coupon *Coupon) error
	Get(ctx context.Context, id string) (*Coupon, error)
	Update(ctx context.Context, coupon *Coupon) error
	Delete(ctx context.Context, id string) error
	IncrementRedemptions(ctx context.Context, id string) error
}
