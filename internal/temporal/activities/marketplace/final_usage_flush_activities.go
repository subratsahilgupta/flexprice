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
// never when it is stored. GCP requires the reported timestamp to be strictly earlier than the
// cancellation instant it holds, and cancelAt is exactly that instant, so a second satisfies the
// comparison without materially under-billing. Applied to every provider rather than branched.
//
// Storing the exact cancelAt keeps the computed-through frontier equal to it after a successful
// write, so a retry finds nothing left to compute rather than a one-second sliver.
const reportTimestampSafetyMargin = 1 * time.Second

// backlogSubmissionWindow bounds the backlog to rows a marketplace can still accept. None of the
// three take a report older than this, so there is no point fetching one.
const backlogSubmissionWindow = 24 * time.Hour

// FlushActivities computes and reports a cancelled subscription's final marketplace usage, then
// archives its marketplace mappings. It runs once per cancellation rather than on a schedule because
// AWS and GCP accept a final report for only about an hour after cancellation, far tighter than the
// reporting crons' cadence.
//
// The per-provider mechanics live on the shared reporter, the same instance the reporting cron holds,
// so the two report paths cannot drift apart.
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

// NewFlushActivities builds the activity set. The reporter is constructed once at registration and
// shared with the reporting cron.
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

// subscriptionMarketplaceMappings returns the subscription's published mappings for the marketplace
// providers.
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

// MarketplaceSubscriptionFinalUsageFlushActivity reports the subscription's outstanding usage in two
// phases — first the backlog of unsynced records, then the final window up to the cancellation — and
// archives its marketplace mappings. The phases are kept separate, and logged separately, so a run's
// logs say which one a line belongs to.
//
// Archiving only happens once every record in both phases has fully synced and every mapped
// connection resolved. On any failure the mappings stay published: that is what keeps the
// subscription's backlog visible to the reporting cron, which retries it independently of this
// activity. Archiving early would strand whatever failed with no way to reach it again.
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

	// One unresolvable connection must not stop the others from being reported, but it does block
	// archiving below: that provider was never attempted, so nothing can be concluded about it.
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

	// Phase 1: the backlog already written by earlier snapshots. Read before phase 2 creates anything,
	// so a first run never picks up its own final record. A retry does — by then a previous attempt's
	// record is just another unsynced row, and phase 2 declines to create a second one.
	backlog, err := a.usageRecordRepo.List(ctx, &types.UsageRecordFilter{
		QueryFilter: &types.QueryFilter{
			Sort:  lo.ToPtr("period_end"),
			Order: lo.ToPtr(types.OrderAsc),
		},
		SubscriptionID: input.SubscriptionID,
		Synced:         lo.ToPtr(false),
		// Bound to the submission window: an older row can no longer be accepted by any provider.
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

	// Phase 2: the single record covering everything from the last computed point up to cancellation.
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

	// An ineligible record — non-USD, or a negative amount — can never be accepted, so it is neither
	// reported nor written. The eligibility check logs which of the two it was.
	finalUsageFlushFailed := false
	if finalUsageFlush != nil && a.reporter.isEligibleForReport(ctx, finalUsageFlush) {
		relevantConns := relevantConnections(finalUsageFlush, preparedConns)
		var reportedConnIDs []string
		if len(relevantConns) > 0 {
			reportedConnIDs = a.reporter.reportRecordToConnections(ctx, finalUsageFlush, relevantConns)
		}
		finalUsageFlushFailed = len(relevantConns) == 0 || !finalUsageFlush.Synced

		if !finalUsageFlushFailed {
			// The providers received cancelAt less the margin; the stored row keeps the true instant.
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
		// Each cause is reported separately: an unresolved connection and an unreported final record
		// both fail the run while leaving no failed record ids behind, so counts alone would describe
		// the failure as affecting zero records and hide what actually went wrong.
		//
		// Returning an error retries the whole activity. That is safe to repeat: connections already
		// holding a sync entry are not attempted again.
		return result, ierr.NewErrorf("marketplace subscription flush failed for subscription %s", input.SubscriptionID).
			WithReportableDetails(map[string]any{
				"subscription_id":              input.SubscriptionID,
				"connection_resolution_failed": connectionResolutionFailed,
				"final_usage_flush_failed":     finalUsageFlushFailed,
				"records_failed":               len(result.FailedRecordIDs),
				"records_succeeded":            len(result.SucceededRecordIDs),
				"records_skipped":              len(result.SkippedRecordIDs),
			}).
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

// reportRecord reports one record to every relevant connection, logs what was accepted, and records
// the outcome on result.
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

// delinkSubscriptionMappings archives the subscription's marketplace mappings and returns the ids it
// archived. It soft-deletes through the repository directly, since the rows are already resolved.
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

// lastComputedPeriodEnd returns the point this subscription's usage has already been computed up to,
// which is where the final window starts: the latest period_end across every published row, whatever
// its sync state. Taking the latest unsynced row instead would re-report a span an already-synced row
// covers, billing it twice.
//
// With no usage record yet, it falls back to the earliest marketplace mapping's creation time — the
// point the subscription became reportable, before which nothing could have been owed.
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

// buildFinalUsageRecord computes the subscription's usage for [windowStart, cancelAt) and returns the
// record to report without writing it: the caller stores it only after a marketplace has accepted it,
// so an unreportable record never reaches the table. Returns nil when cancelAt is at or before
// windowStart and there is nothing left to compute — a backdated cancellation, or a retry after an
// earlier attempt already recorded this window.
//
// The amount covers the window through cancelAt, but PeriodEnd on the returned record carries the
// reporting margin already applied; the caller restores the true instant before storing it.
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

// createFinalUsageRecord stores the final record after its marketplaces have accepted it. rec already
// carries the sync state that reporting produced.
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
