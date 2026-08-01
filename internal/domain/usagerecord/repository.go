package usagerecord

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// Repository provides persistence for usage records.
type Repository interface {
	// Create inserts a new usage record.
	Create(ctx context.Context, record *UsageRecord) error

	// ExistsForPeriod reports whether a usage record already exists for this subscription and
	// exact reporting window. Used to make the snapshot activity idempotent under Temporal
	// retries: the window is deterministic per scheduled run, so a matching row means this
	// subscription was already processed by an earlier attempt.
	ExistsForPeriod(ctx context.Context, subscriptionID string, periodStart, periodEnd time.Time) (bool, error)

	// ListUnsynced returns this tenant/environment's usage records that are not yet fully synced
	// (synced=false) — not scoped to any one connection, since a record can be relevant to several.
	// Bounded to period_end within the last 24h: none of the three marketplaces accept a report
	// older than that, so an older row is never re-fetched (ERD FLE-1106 §5.2).
	ListUnsynced(ctx context.Context, tenantID, environmentID string) ([]*UsageRecord, error)

	// List returns usage records matching filter — subscription-scoped queries (the cancellation
	// flush's frontier and backlog lookups) go through this rather than a bespoke method per query.
	List(ctx context.Context, filter *types.UsageRecordFilter) ([]*UsageRecord, error)

	// MarkSynced writes the record's syncs map (one entry per connection it's been reported to) and
	// the synced flag, which the caller sets true once every connection relevant to this record has
	// an entry. The reporting cron builds the map in memory and calls this once per record.
	MarkSynced(ctx context.Context, id string, syncs map[string]types.UsageRecordSyncEntry, synced bool) error
}
