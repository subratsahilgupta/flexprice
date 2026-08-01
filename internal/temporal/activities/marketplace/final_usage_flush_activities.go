package marketplace

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/connection"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/domain/usagerecord"
	"github.com/flexprice/flexprice/internal/ee/service"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	temporalModels "github.com/flexprice/flexprice/internal/temporal/models"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"go.temporal.io/sdk/activity"
)

// reportTimestampSafetyMargin is subtracted from the final record's period_end when it is reported,
// never when it is stored. GCP requires the reported timestamp to be strictly before its own recorded
// cancellation instant, and CancelAt is exactly that instant (sub.CancelAt, never sub.CancelledAt —
// see ERD FLE-1106 §3.8), so one second satisfies the strict "<" without materially under-billing.
// Applied uniformly to all three providers rather than branched per provider.
//
// Storing the exact cancelAt also keeps lastComputedPeriodEnd's MAX(period_end) frontier equal to
// cancelAt after a successful write, so a retry computes no leftover sliver window.
const reportTimestampSafetyMargin = 1 * time.Second

// backlogSubmissionWindow mirrors ListUnsynced's own bound (repository/ent/usagerecord.go) — none of
// the three marketplaces accept a report older than this, so there's no reason to fetch it here either.
const backlogSubmissionWindow = 24 * time.Hour

// FlushActivities computes and reports a cancelled subscription's final marketplace usage. It is
// started once per cancellation from CancelSubscription (post-commit, non-blocking) — not on a
// schedule — because AWS and GCP only accept a final report within roughly an hour of cancellation,
// far tighter than the ordinary 6h snapshot / 3h report cron cadence (ERD FLE-1106 §4).
//
// The per-provider mechanics live on the shared marketplaceReporter, the same instance the scheduled
// reporting cron holds, so the two report paths can never drift out of sync with each other.
type FlushActivities struct {
	subscriptionService          service.SubscriptionService
	billingService               service.BillingService
	connectionRepo               connection.Repository
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	subscriptionRepo             subscription.Repository
	customerRepo                 customer.Repository
	usageRecordRepo              usagerecord.Repository
	reporter                     *marketplaceReporter
	logger                       *logger.Logger
}

// NewFlushActivities takes ServiceParams rather than each repo individually (the pattern used by
// NewInvoiceSyncActivities and friends) — every dependency below already lives on it. The reporter is
// the same instance the scheduled reporting cron holds, so both report through identical per-provider
// code and cannot drift apart.
func NewFlushActivities(params service.ServiceParams, reporter *marketplaceReporter, log *logger.Logger) *FlushActivities {
	return &FlushActivities{
		subscriptionService:          service.NewSubscriptionService(params),
		billingService:               service.NewBillingService(params),
		connectionRepo:               params.ConnectionRepo,
		entityIntegrationMappingRepo: params.EntityIntegrationMappingRepo,
		subscriptionRepo:             params.SubRepo,
		customerRepo:                 params.CustomerRepo,
		usageRecordRepo:              params.UsageRecordRepo,
		reporter:                     reporter,
		logger:                       log,
	}
}

// subscriptionMarketplaceMappings resolves the published marketplace mappings for a subscription,
// scoped to the same three providers the snapshot/report crons iterate.
func (a *FlushActivities) subscriptionMarketplaceMappings(ctx context.Context, subscriptionID string) ([]*entityintegrationmapping.EntityIntegrationMapping, error) {
	providerTypes := make([]string, len(marketplaceProviderTypes))
	for i, p := range marketplaceProviderTypes {
		providerTypes[i] = string(p)
	}
	return a.entityIntegrationMappingRepo.List(ctx, &types.EntityIntegrationMappingFilter{
		QueryFilter:   types.NewNoLimitPublishedQueryFilter(),
		EntityID:      subscriptionID,
		EntityType:    types.IntegrationEntityTypeSubscription,
		ProviderTypes: providerTypes,
	})
}

// MarketplaceSubscriptionFinalUsageFlushActivity reports the pre-existing backlog of unsynced records,
// then computes, writes and reports the final usage window, then archives the subscription's
// marketplace mapping(s). The backlog and the final record are two explicit, separately logged phases
// — not merged into one query — so a reader of the logs always knows which one they're looking at.
//
// The delink only runs once every record across both phases has fully synced and every mapped
// connection could be resolved. On any failure the mapping stays published and nothing else is done
// about it here — there is no dedicated retry for a failed flush today beyond this activity's own
// Temporal retries (up to 5 attempts). Leaving the mapping published is what keeps the subscription's
// backlog visible to the scheduled report cron, which will keep retrying it independently of this
// activity; a purpose-built catch-up mechanism for a flush that exhausted its retries is a known gap,
// not yet built.
func (a *FlushActivities) MarketplaceSubscriptionFinalUsageFlushActivity(
	ctx context.Context,
	input temporalModels.MarketplaceSubscriptionFinalUsageFlushActivityInput,
) (*temporalModels.MarketplaceSubscriptionFinalUsageFlushWorkflowResult, error) {
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	log := activity.GetLogger(ctx)
	tenantID := input.TenantID
	environmentID := input.EnvironmentID
	result := &temporalModels.MarketplaceSubscriptionFinalUsageFlushWorkflowResult{
		SubscriptionID: input.SubscriptionID,
	}

	subMappings, err := a.subscriptionMarketplaceMappings(ctx, input.SubscriptionID)
	if err != nil {
		a.logger.Error(ctx, "marketplace subscription flush failed",
			"tenant_id", tenantID, "environment_id", environmentID, "subscription_id", input.SubscriptionID,
			"error", err, "stage", "list_subscription_mappings")
		return nil, err
	}
	if len(subMappings) == 0 {
		log.Info("subscription has no marketplace mapping, nothing to flush", "subscription_id", input.SubscriptionID)
		return result, nil
	}

	// A connection that fails to resolve for one mapping must not stop the other mappings from being
	// reported — but it does block delink below (via connectionResolutionFailed), since this provider
	// was never actually attempted this run.
	preparedConns := make([]*preparedConnection, 0, len(subMappings))
	connectionResolutionFailed := false
	for _, m := range subMappings {
		conn, err := a.connectionRepo.GetByProvider(ctx, types.SecretProvider(m.ProviderType))
		if err != nil {
			a.logger.Error(ctx, "marketplace subscription flush failed",
				"tenant_id", tenantID, "environment_id", environmentID, "subscription_id", input.SubscriptionID,
				"provider_type", m.ProviderType, "error", err, "stage", "get_connection")
			connectionResolutionFailed = true
			continue
		}
		prepared, err := a.reporter.prepareConnection(ctx, conn)
		if err != nil {
			// already logged inside prepareConnection at the stage that failed
			connectionResolutionFailed = true
			continue
		}
		preparedConns = append(preparedConns, prepared)
	}

	// Phase 1: report the backlog — every unsynced record the snapshot cron already wrote. Fetched
	// before phase 2 creates anything, so a first run never sees its own final record here. On a
	// Temporal retry it does: the final record the previous attempt wrote is by then just another
	// unsynced row, and phase 2 will decline to create a second one.
	backlog, err := a.usageRecordRepo.List(ctx, &types.UsageRecordFilter{
		QueryFilter: &types.QueryFilter{
			Sort:  lo.ToPtr("period_end"),
			Order: lo.ToPtr(types.OrderAsc),
		},
		SubscriptionID: input.SubscriptionID,
		Synced:         lo.ToPtr(false),
		// Bound to the marketplaces' submission window: a row older than this can no longer be
		// accepted by any of the three providers (ERD FLE-1106 §5.2).
		Filters: []*types.FilterCondition{{
			Field:    lo.ToPtr("period_end"),
			Operator: lo.ToPtr(types.GREATER_THAN_EQUAL),
			DataType: lo.ToPtr(types.DataTypeDate),
			Value:    &types.Value{Date: lo.ToPtr(time.Now().UTC().Add(-backlogSubmissionWindow))},
		}},
	})
	if err != nil {
		a.logger.Error(ctx, "marketplace subscription flush failed",
			"tenant_id", tenantID, "environment_id", environmentID, "subscription_id", input.SubscriptionID,
			"error", err, "stage", "list_backlog")
		return nil, err
	}

	for _, rec := range backlog {
		if !a.reporter.isEligibleForReport(ctx, rec) {
			continue
		}
		a.reportRecord(ctx, rec, preparedConns, result)
	}

	// Phase 2: this subscription's one final usage record — computed and reported as its own step.
	cancelAt := input.CancelAt.UTC()

	computedThrough, err := a.lastComputedPeriodEnd(ctx, input.SubscriptionID, subMappings)
	if err != nil {
		a.logger.Error(ctx, "marketplace subscription flush failed",
			"tenant_id", tenantID, "environment_id", environmentID, "subscription_id", input.SubscriptionID,
			"error", err, "stage", "last_computed_period_end")
		return nil, err
	}

	finalUsageFlush, err := a.buildFinalUsageRecord(ctx, input.SubscriptionID, computedThrough, cancelAt, environmentID)
	if err != nil {
		return nil, err
	}

	// An ineligible final record (non-USD, or a negative amount) can never be accepted by any
	// marketplace, so it is not reported and not written — same rule the report cron applies to the
	// backlog, and isEligibleForReport logs which of the two it was.
	finalUsageFlushFailed := false
	if finalUsageFlush != nil && a.reporter.isEligibleForReport(ctx, finalUsageFlush) {
		relevantConns := relevantConnections(finalUsageFlush, preparedConns)
		var reportedConnIDs []string
		if len(relevantConns) > 0 {
			reportedConnIDs = a.reporter.reportRecordToConnections(ctx, finalUsageFlush, relevantConns)
		}
		finalUsageFlushFailed = len(relevantConns) == 0 || !finalUsageFlush.Synced

		if !finalUsageFlushFailed {
			// The wire carried cancelAt minus the margin; the row keeps the true instant.
			finalUsageFlush.PeriodEnd = cancelAt
			if err := a.createFinalUsageRecord(ctx, finalUsageFlush); err != nil {
				return nil, err
			}
			a.logReported(ctx, finalUsageFlush, reportedConnIDs)

			result.FinalRecordID = finalUsageFlush.ID
			result.PeriodStart = finalUsageFlush.PeriodStart
			result.PeriodEnd = finalUsageFlush.PeriodEnd
			if anyRealEntry(finalUsageFlush, relevantConns) {
				result.SucceededRecordIDs = append(result.SucceededRecordIDs, finalUsageFlush.ID)
			} else {
				result.SkippedRecordIDs = append(result.SkippedRecordIDs, finalUsageFlush.ID)
			}
		}
	}

	if connectionResolutionFailed || finalUsageFlushFailed || len(result.FailedRecordIDs) > 0 {
		// Mapping stays published — see this function's doc comment. Returned so Temporal retries the
		// whole activity; the syncs map already written makes a retry idempotent, since only the
		// connections still missing an entry are re-attempted.
		return result, ierr.NewErrorf("marketplace subscription flush: %d of %d records did not fully sync",
			len(result.FailedRecordIDs), len(result.SucceededRecordIDs)+len(result.FailedRecordIDs)+len(result.SkippedRecordIDs)).
			WithReportableDetails(map[string]any{"subscription_id": input.SubscriptionID}).
			Mark(ierr.ErrInternal)
	}

	delinkedIDs, err := a.delinkSubscriptionMappings(ctx, subMappings)
	if err != nil {
		return result, err
	}
	result.DelinkedMappingIDs = delinkedIDs

	log.Info("MarketplaceSubscriptionFinalUsageFlushActivity completed",
		"subscription_id", input.SubscriptionID,
		"final_record_id", result.FinalRecordID,
		"records_succeeded", len(result.SucceededRecordIDs),
		"records_skipped", len(result.SkippedRecordIDs),
		"mappings_delinked", len(result.DelinkedMappingIDs))
	return result, nil
}

// reportRecord reports one record to every relevant connection, logs what happened, and records the
// outcome on result. Mirrors ReportActivities.reportRecord on the cron side.
func (a *FlushActivities) reportRecord(
	ctx context.Context,
	rec *usagerecord.UsageRecord,
	preparedConns []*preparedConnection,
	result *temporalModels.MarketplaceSubscriptionFinalUsageFlushWorkflowResult,
) {
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	relevantConns := relevantConnections(rec, preparedConns)
	if len(relevantConns) == 0 {
		return // no resolved connection is mapped to this subscription
	}
	reportedConnIDs := a.reporter.reportRecordToConnections(ctx, rec, relevantConns)
	a.logReported(ctx, rec, reportedConnIDs)

	if err := a.usageRecordRepo.MarkSynced(ctx, rec.ID, rec.Syncs, rec.Synced); err != nil {
		a.logger.Error(ctx, "marketplace subscription flush failed",
			"tenant_id", tenantID, "environment_id", environmentID, "subscription_id", rec.SubscriptionID,
			"usage_record_id", rec.ID, "error", err, "stage", "mark_synced")
		rec.Synced = false
	}

	switch {
	case !rec.Synced:
		result.FailedRecordIDs = append(result.FailedRecordIDs, rec.ID)
	case anyRealEntry(rec, relevantConns):
		result.SucceededRecordIDs = append(result.SucceededRecordIDs, rec.ID)
	default:
		result.SkippedRecordIDs = append(result.SkippedRecordIDs, rec.ID)
	}
}

// logReported logs one line per connection that accepted this record on this run.
func (a *FlushActivities) logReported(ctx context.Context, rec *usagerecord.UsageRecord, reportedConnIDs []string) {
	for _, connectionID := range reportedConnIDs {
		entry := rec.Syncs[connectionID]
		a.logger.Info(ctx, "marketplace subscription flush: usage record synced",
			"tenant_id", types.GetTenantID(ctx), "environment_id", types.GetEnvironmentID(ctx),
			"subscription_id", rec.SubscriptionID, "usage_record_id", rec.ID,
			"connection_id", connectionID, "marketplace", entry.Marketplace,
			"reporting_id", entry.ReportingID)
	}
}

// delinkSubscriptionMappings archives the subscription's marketplace mappings, returning how many
// were archived. Uses the repository's soft-delete directly rather than the service's
// DelinkIntegrationMapping, which would re-query for the same rows already resolved here.
func (a *FlushActivities) delinkSubscriptionMappings(ctx context.Context, subMappings []*entityintegrationmapping.EntityIntegrationMapping) ([]string, error) {
	delinked := make([]string, 0, len(subMappings))
	for _, m := range subMappings {
		if err := a.entityIntegrationMappingRepo.Delete(ctx, m); err != nil {
			a.logger.Error(ctx, "marketplace subscription flush failed",
				"tenant_id", types.GetTenantID(ctx), "environment_id", types.GetEnvironmentID(ctx),
				"subscription_id", m.EntityID, "provider_type", m.ProviderType,
				"error", err, "stage", "delink_mapping")
			return delinked, err
		}
		delinked = append(delinked, m.ID)
	}
	return delinked, nil
}

// lastComputedPeriodEnd returns the point this subscription's usage has already been computed up to:
// the latest period_end across every published usage_record, regardless of sync state. Using only
// unsynced rows would risk re-reporting a span an earlier, already-synced row already covered
// (ERD FLE-1106 §3.3). It is the start of the final flush window.
//
// Falls back to the earliest of the subscription's marketplace mappings' own created_at when no
// usage_record exists yet (ERD §3.4): a mapping's creation is when this subscription became
// reportable to that marketplace, so nothing before it could ever have been owed there.
func (a *FlushActivities) lastComputedPeriodEnd(ctx context.Context, subscriptionID string, subMappings []*entityintegrationmapping.EntityIntegrationMapping) (time.Time, error) {
	rows, err := a.usageRecordRepo.List(ctx, &types.UsageRecordFilter{
		QueryFilter: &types.QueryFilter{
			Sort:  lo.ToPtr("period_end"),
			Order: lo.ToPtr(types.OrderDesc),
			Limit: lo.ToPtr(1),
		},
		SubscriptionID: subscriptionID,
	})
	if err != nil {
		return time.Time{}, err
	}
	if len(rows) > 0 {
		return rows[0].PeriodEnd, nil
	}

	earliest := subMappings[0].CreatedAt
	for _, m := range subMappings[1:] {
		if m.CreatedAt.Before(earliest) {
			earliest = m.CreatedAt
		}
	}
	return earliest, nil
}

// buildFinalUsageRecord computes the subscription's final usage for [windowStart, cancelAt) and
// returns the record to report, without writing it — the caller inserts it only once a marketplace
// has accepted it (see createFinalUsageRecord). Returns nil when there is nothing left to compute:
// cancelAt at or before windowStart, which is what a backdated cancellation looks like, and also what
// a Temporal retry sees once an earlier attempt already wrote the record.
//
// The usage amount covers the true window up to cancelAt, but PeriodEnd on the returned record is the
// wire timestamp — cancelAt minus the reporting margin. The caller restores the true instant before
// storing it.
func (a *FlushActivities) buildFinalUsageRecord(ctx context.Context, subscriptionID string, windowStart, cancelAt time.Time, environmentID string) (*usagerecord.UsageRecord, error) {
	if !cancelAt.After(windowStart) {
		return nil, nil
	}

	tenantID := types.GetTenantID(ctx)

	sub, _, err := a.subscriptionRepo.GetWithLineItems(ctx, subscriptionID)
	if err != nil {
		a.logger.Error(ctx, "marketplace subscription flush failed",
			"tenant_id", tenantID, "environment_id", environmentID, "subscription_id", subscriptionID,
			"error", err, "stage", "get_subscription")
		return nil, err
	}

	usageResp, err := a.subscriptionService.GetMeterUsageBySubscription(ctx, &dto.GetUsageBySubscriptionRequest{
		SubscriptionID: subscriptionID,
		StartTime:      windowStart,
		EndTime:        cancelAt,
		Source:         string(types.UsageSourceInvoiceCreation),
	})
	if err != nil {
		a.logger.Error(ctx, "marketplace subscription flush failed",
			"tenant_id", tenantID, "environment_id", environmentID, "subscription_id", subscriptionID,
			"customer_id", sub.CustomerID, "error", err, "stage", "get_meter_usage")
		return nil, err
	}

	_, totalAmount, err := a.billingService.CalculateMeterUsageCharges(
		ctx, sub, usageResp, windowStart, cancelAt, types.UsageSourceInvoiceCreation,
	)
	if err != nil {
		a.logger.Error(ctx, "marketplace subscription flush failed",
			"tenant_id", tenantID, "environment_id", environmentID, "subscription_id", subscriptionID,
			"customer_id", sub.CustomerID, "error", err, "stage", "calculate_charges")
		return nil, err
	}

	customerExternalID := ""
	if cust, custErr := a.customerRepo.Get(ctx, sub.CustomerID); custErr == nil && cust != nil {
		customerExternalID = cust.ExternalID
	}

	return &usagerecord.UsageRecord{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_USAGE_RECORD),
		CustomerID:         sub.CustomerID,
		CustomerExternalID: customerExternalID,
		SubscriptionID:     sub.ID,
		PlanID:             sub.PlanID,
		Amount:             totalAmount,
		Currency:           usageResp.Currency,
		PeriodStart:        windowStart,
		PeriodEnd:          cancelAt.Add(-reportTimestampSafetyMargin),
		EnvironmentID:      environmentID,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}, nil
}

// createFinalUsageRecord writes the final record once its marketplaces have accepted it. rec already
// carries the sync state the report produced.
func (a *FlushActivities) createFinalUsageRecord(ctx context.Context, rec *usagerecord.UsageRecord) error {
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	if err := a.usageRecordRepo.Create(ctx, rec); err != nil {
		// A concurrent attempt can win the race between computing this window and inserting it. That
		// attempt reported the same usage and wrote the row, so this one has nothing left to do.
		if ierr.IsAlreadyExists(err) {
			return nil
		}
		a.logger.Error(ctx, "marketplace subscription flush failed",
			"tenant_id", tenantID, "environment_id", environmentID,
			"subscription_id", rec.SubscriptionID, "customer_id", rec.CustomerID,
			"usage_record_id", rec.ID, "error", err, "stage", "create_usage_record")
		return err
	}

	a.logger.Info(ctx, "marketplace subscription flush: final usage record created",
		"tenant_id", tenantID, "environment_id", environmentID,
		"subscription_id", rec.SubscriptionID, "customer_id", rec.CustomerID, "usage_record_id", rec.ID,
		"period_start", rec.PeriodStart, "period_end", rec.PeriodEnd, "amount", rec.Amount,
		"synced", rec.Synced)
	return nil
}
