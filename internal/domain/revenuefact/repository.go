package revenuefact

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// Repository persists and retrieves revenue_facts rows.
type Repository interface {
	// UpsertProvisional inserts/updates facts on the provisional grain
	// (tenant, environment, subscription, price, day, revenue_source),
	// bumping version on conflict.
	UpsertProvisional(ctx context.Context, facts []*RevenueFact) error

	// FlipToFinal converts PROVISIONAL rows for a subscription/price/period to
	// FINAL, stamping the given invoice line item. Returns the rows affected.
	FlipToFinal(ctx context.Context, subscriptionID, priceID string, periodStart, periodEnd time.Time, invoiceID, invoiceLineItemID string) (int, error)

	// ListBySubscriptionPeriod lists facts for a subscription within a period, filtered by status.
	ListBySubscriptionPeriod(ctx context.Context, subscriptionID string, periodStart, periodEnd time.Time, status types.FactStatus) ([]*RevenueFact, error)
}
