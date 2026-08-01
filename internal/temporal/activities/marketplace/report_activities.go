package marketplace

import (
	"context"

	"github.com/flexprice/flexprice/internal/domain/connection"
	"github.com/flexprice/flexprice/internal/domain/usagerecord"
	"github.com/flexprice/flexprice/internal/ee/service"
	"github.com/flexprice/flexprice/internal/logger"
	temporalModels "github.com/flexprice/flexprice/internal/temporal/models"
	"github.com/flexprice/flexprice/internal/types"
	"go.temporal.io/sdk/activity"
)

// ReportActivities reports usage records that have not yet reached every marketplace connection
// relevant to them. For every tenant/environment with a published marketplace connection it
// authenticates each connection once, reads that tenant's unsynced usage records once, and reports
// each record to whichever relevant connections it hasn't already reached. A record a marketplace
// rejects is left unsynced so the next run retries it; there is no terminal failure state.
//
// The per-provider mechanics live on the shared marketplaceReporter, the same instance the
// cancellation flush holds, so the two cannot drift apart.
type ReportActivities struct {
	reporter        *marketplaceReporter
	connectionRepo  connection.Repository
	usageRecordRepo usagerecord.Repository
	logger          *logger.Logger
}

// NewReportActivities takes ServiceParams for its own repos (the pattern used by
// NewInvoiceSyncActivities and friends); the reporter is constructed once in registration and shared
// with FlushActivities.
func NewReportActivities(params service.ServiceParams, reporter *marketplaceReporter, log *logger.Logger) *ReportActivities {
	return &ReportActivities{
		reporter:        reporter,
		connectionRepo:  params.ConnectionRepo,
		usageRecordRepo: params.UsageRecordRepo,
		logger:          log,
	}
}

// MarketplaceUsageReportActivity is the activity entrypoint. A record's relevant connections and its
// unsynced status are both tenant/environment-scoped, so it groups every published marketplace
// connection by tenant/environment and reports each group independently — a failure in one tenant
// does not stop the others.
func (a *ReportActivities) MarketplaceUsageReportActivity(
	ctx context.Context,
	_ temporalModels.MarketplaceUsageReportWorkflowInput,
) (*temporalModels.MarketplaceUsageReportWorkflowResult, error) {
	log := activity.GetLogger(ctx)
	log.Info("Starting MarketplaceUsageReportActivity")

	result := &temporalModels.MarketplaceUsageReportWorkflowResult{}

	type tenantEnv struct{ tenantID, environmentID string }
	connsByTenant := make(map[tenantEnv][]*connection.Connection)
	for _, providerType := range marketplaceProviderTypes {
		conns, err := a.connectionRepo.ListPublishedByProvider(ctx, providerType)
		if err != nil {
			a.logger.Error(ctx, "marketplace usage report failed", "marketplace", providerType, "error", err, "stage", "list_connections")
			continue
		}
		for _, conn := range conns {
			key := tenantEnv{conn.TenantID, conn.EnvironmentID}
			connsByTenant[key] = append(connsByTenant[key], conn)
		}
	}

	for key, conns := range connsByTenant {
		ctx := types.SetTenantID(ctx, key.tenantID)
		ctx = types.SetEnvironmentID(ctx, key.environmentID)
		a.reportForTenant(ctx, key.tenantID, key.environmentID, conns, result)
	}

	log.Info("Completed MarketplaceUsageReportActivity",
		"total", result.Total, "succeeded", result.Succeeded, "failed", result.Failed)
	return result, nil
}

// reportForTenant authenticates each of this tenant/environment's published connections once,
// fetches its unsynced usage records once, and reports each record to the relevant ones. Both lists
// are fixed for the whole call — nothing is re-queried mid-run, so a connection deleted while this
// run executes only takes effect from the next scheduled run.
func (a *ReportActivities) reportForTenant(
	ctx context.Context,
	tenantID, environmentID string,
	conns []*connection.Connection,
	result *temporalModels.MarketplaceUsageReportWorkflowResult,
) {
	preparedConns := make([]*preparedConnection, 0, len(conns))
	for _, conn := range conns {
		preparedConn, err := a.reporter.prepareConnection(ctx, conn)
		if err != nil {
			continue // already logged inside prepareConnection at the stage that failed
		}
		preparedConns = append(preparedConns, preparedConn)
	}
	if len(preparedConns) == 0 {
		return
	}

	records, err := a.usageRecordRepo.ListUnsynced(ctx, tenantID, environmentID)
	if err != nil {
		a.logger.Error(ctx, "marketplace usage report failed",
			"tenant_id", tenantID, "environment_id", environmentID, "error", err, "stage", "list_unsynced")
		return
	}

	for _, rec := range records {
		if a.reporter.isEligibleForReport(ctx, rec) {
			a.reportRecord(ctx, rec, preparedConns, result)
		}
	}
}

// reportRecord reports one record to every connection relevant to its subscription and classifies the
// result into exactly one of Succeeded/Failed/Skipped. The per-connection mechanics and the
// classification rule live on the shared reporter (reportRecordToConnections) so the cron and the
// cancellation flush can never disagree on what "succeeded" means; this function only logs and
// bookkeeps in its own words.
func (a *ReportActivities) reportRecord(
	ctx context.Context,
	rec *usagerecord.UsageRecord,
	preparedConns []*preparedConnection,
	result *temporalModels.MarketplaceUsageReportWorkflowResult,
) {
	relevantConns := relevantConnections(rec, preparedConns)
	if len(relevantConns) == 0 {
		return // no published connection is mapped to this subscription right now
	}
	reportedConnIDs := a.reporter.reportRecordToConnections(ctx, rec, relevantConns)

	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)
	for _, connectionID := range reportedConnIDs {
		entry := rec.Syncs[connectionID]
		a.logger.Info(ctx, "marketplace usage record synced",
			"tenant_id", tenantID, "environment_id", environmentID, "subscription_id", rec.SubscriptionID,
			"usage_record_id", rec.ID, "connection_id", connectionID,
			"marketplace", entry.Marketplace, "reporting_id", entry.ReportingID)
	}

	if err := a.usageRecordRepo.MarkSynced(ctx, rec.ID, rec.Syncs, rec.Synced); err != nil {
		a.logger.Error(ctx, "marketplace usage report failed",
			"tenant_id", tenantID, "environment_id", environmentID,
			"subscription_id", rec.SubscriptionID, "usage_record_id", rec.ID, "error", err, "stage", "mark_synced")
		rec.Synced = false
	}

	result.Total++
	switch {
	case !rec.Synced:
		result.AppendFailedRecordID(rec.ID)
	case anyRealEntry(rec, relevantConns):
		result.AppendSucceededRecordID(rec.ID)
	default:
		result.AppendSkippedRecordID(rec.ID)
	}
}

// isEligibleForReport filters out records that must never reach a marketplace at all: non-USD
// currency (none of the three marketplaces accept it) and negative amounts (a credit, not usage).
func (r *marketplaceReporter) isEligibleForReport(ctx context.Context, rec *usagerecord.UsageRecord) bool {
	if !types.IsMatchingCurrency(rec.Currency, marketplaceReportingCurrency) {
		r.logger.Debug(ctx, "skipping marketplace usage record, currency not usd",
			"subscription_id", rec.SubscriptionID, "usage_record_id", rec.ID, "currency", rec.Currency)
		return false
	}
	if rec.Amount.IsNegative() {
		r.logger.Error(ctx, "marketplace usage record has negative amount",
			"subscription_id", rec.SubscriptionID, "usage_record_id", rec.ID, "amount", rec.Amount,
			"error", "negative_amount")
		return false
	}
	return true
}
