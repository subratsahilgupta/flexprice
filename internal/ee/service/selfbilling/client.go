package selfbilling

import (
	"context"
	"fmt"
	"time"
)

// billingClient is the slice of the Mumbai billing tenant API self-billing uses.
type billingClient interface {
	EnsureCustomer(ctx context.Context, in customerInput) error
	StartTrial(ctx context.Context, extID string) error
	// LatestSubscription returns the newest non-cancelled self-serve subscription, else the newest cancelled one, else nil.
	LatestSubscription(ctx context.Context, extID string) (*remoteSubscription, error)
}

type customerInput struct {
	extID, legacyExtID, name, email, tenantID string
}

type remoteSubscription struct {
	id       string
	status   string // trialing, active, incomplete, cancelled, paused, draft
	trialEnd *time.Time
}

func externalID(region, tenantID string) string {
	return fmt.Sprintf("%s:%s", region, tenantID)
}
