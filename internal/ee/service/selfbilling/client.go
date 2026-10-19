package selfbilling

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/types"
)

// billingClient is the slice of the Flexprice Cloud API self-billing uses.
type billingClient interface {
	// EnsureCustomer finds or creates the tenant's customer on Flexprice Cloud.
	EnsureCustomer(ctx context.Context, in customerInput) error

	// StartTrial subscribes the customer to the self-serve plan unless it already has a subscription to it.
	StartTrial(ctx context.Context, extID string) error

	// LatestSubscription returns the newest non-cancelled self-serve subscription, else the newest cancelled one, else nil.
	LatestSubscription(ctx context.Context, extID string) (*remoteSubscription, error)
}

type customerInput struct {
	extID       string
	legacyExtID string
	name        string
	email       string
	tenantID    string
}

type remoteSubscription struct {
	id     string
	status types.SubscriptionStatus
}

func externalID(region, tenantID string) string {
	return fmt.Sprintf("%s:%s", region, tenantID)
}
