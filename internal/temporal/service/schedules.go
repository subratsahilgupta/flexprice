package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/temporal/client"
	"github.com/flexprice/flexprice/internal/temporal/models"
	invoiceModels "github.com/flexprice/flexprice/internal/temporal/models/invoice"
	subscriptionModels "github.com/flexprice/flexprice/internal/temporal/models/subscription"
	cronWorkflows "github.com/flexprice/flexprice/internal/temporal/workflows/cron"
	invoiceWorkflows "github.com/flexprice/flexprice/internal/temporal/workflows/invoice"
	subscriptionWorkflows "github.com/flexprice/flexprice/internal/temporal/workflows/subscription"
	"github.com/flexprice/flexprice/internal/types"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"
)

// AllTemporalScheduleConfigs returns the configuration for every Temporal server schedule
// (not HTTP-only cron entrypoints; keep in sync with types.AllTemporalServerScheduleIDs).
// cfg may be nil (schedules that don't need it, like this one, still get sane defaults).
func AllTemporalScheduleConfigs(cfg *config.Configuration) []types.ScheduleConfig {
	revenueRollupInterval := defaultRevenueRollupScheduleInterval
	if cfg != nil && cfg.Analytics.RevenueRollup.Interval > 0 {
		revenueRollupInterval = cfg.Analytics.RevenueRollup.Interval
	}

	return []types.ScheduleConfig{
		{
			ID:        types.ScheduleIDCreditGrantProcessing,
			Interval:  15 * time.Minute,
			Workflow:  cronWorkflows.CreditGrantProcessingWorkflow,
			Input:     models.CreditGrantProcessingWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDSubscriptionAutoCancellation,
			Interval:  15 * time.Minute,
			Workflow:  cronWorkflows.SubscriptionAutoCancellationWorkflow,
			Input:     models.SubscriptionAutoCancellationWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDWalletCreditExpiry,
			Interval:  15 * time.Minute,
			Workflow:  cronWorkflows.WalletCreditExpiryWorkflow,
			Input:     models.WalletCreditExpiryWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDSubscriptionBilling,
			Interval:  2 * time.Minute,
			Workflow:  subscriptionWorkflows.ScheduleSubscriptionBillingWorkflow,
			Input:     subscriptionModels.ScheduleSubscriptionBillingWorkflowInput{BatchSize: types.DEFAULT_BATCH_SIZE},
			TaskQueue: types.TemporalTaskQueueSubscription,
		},
		{
			ID:        types.ScheduleIDSubscriptionRenewalAlerts,
			Interval:  15 * time.Minute,
			Workflow:  cronWorkflows.SubscriptionRenewalDueAlertsWorkflow,
			Input:     models.SubscriptionRenewalDueAlertsWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDSubscriptionTrialEndDue,
			Interval:  15 * time.Minute,
			Workflow:  cronWorkflows.SubscriptionTrialEndDueWorkflow,
			Input:     models.SubscriptionTrialEndDueWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDSubscriptionAutoInvoiceThresholdBilling,
			Interval:  5 * time.Minute,
			Workflow:  cronWorkflows.AutoInvoiceThresholdBillingWorkflow,
			Input:     models.AutoInvoiceThresholdBillingWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDOutboundWebhookStaleRetry,
			Interval:  2 * time.Minute,
			Workflow:  cronWorkflows.OutboundWebhookStaleRetryWorkflow,
			Input:     models.OutboundWebhookStaleRetryWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDPaddleInvoicePullSync,
			Interval:  1 * time.Hour,
			Workflow:  cronWorkflows.PaddleInvoicePullSyncCronWorkflow,
			Input:     models.PaddleInvoicePullSyncCronInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDMoyasarAuthPaymentSettlement,
			Interval:  15 * time.Minute,
			Workflow:  cronWorkflows.MoyasarAuthPaymentSettlementWorkflow,
			Input:     struct{}{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDCheckoutSessionExpiry,
			Interval:  30 * time.Minute,
			Workflow:  cronWorkflows.CheckoutSessionExpiryWorkflow,
			Input:     models.CheckoutSessionExpiryWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDMarketplaceUsageSnapshot,
			Interval:  6 * time.Hour,
			Workflow:  cronWorkflows.MarketplaceUsageSnapshotWorkflow,
			Input:     models.MarketplaceUsageSnapshotWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDMarketplaceUsageReport,
			Interval:  3 * time.Hour,
			Workflow:  cronWorkflows.MarketplaceUsageReportWorkflow,
			Input:     models.MarketplaceUsageReportWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			ID:        types.ScheduleIDDailyDraftAndCompute,
			Interval:  24 * time.Hour,
			Offset:    2 * time.Hour, // fires at 02:00 UTC daily
			Workflow:  cronWorkflows.DailyDraftAndComputeWorkflow,
			Input:     models.DailyDraftAndComputeWorkflowInput{},
			TaskQueue: types.TemporalTaskQueueCron,
		},
		{
			// Fans out FinalizeDraftInvoiceWorkflow for each draft whose finalization delay has elapsed.
			// Runs on the invoice queue (where ScheduleDraftFinalizationWorkflow is registered), not the cron queue.
			ID:        types.ScheduleIDDraftInvoiceFinalization,
			Interval:  30 * time.Minute,
			Workflow:  invoiceWorkflows.ScheduleDraftFinalizationWorkflow,
			Input:     invoiceModels.ScheduleDraftFinalizationWorkflowInput{BatchSize: types.DEFAULT_BATCH_SIZE},
			TaskQueue: types.TemporalTaskQueueInvoice,
		},
		{
			// Off by default — see analytics.revenue_rollup.enabled and isScheduleEnabled.
			ID:        types.ScheduleIDRevenueRollup,
			Interval:  revenueRollupInterval,
			Workflow:  cronWorkflows.RevenueRollupWorkflow,
			Input:     models.RevenueRollupInput{Interval: revenueRollupInterval},
			TaskQueue: types.TemporalTaskQueueCron,
		},
	}
}

// defaultRevenueRollupScheduleInterval is used when analytics.revenue_rollup.interval is unset.
const defaultRevenueRollupScheduleInterval = time.Hour

// EnsureSchedules idempotently creates or updates every configured Temporal server schedule
// that isScheduleEnabled allows. A disabled schedule is skipped entirely — no Describe/Create/Update
// call is made for it — so booting with a flag off never touches the Temporal server for that schedule.
// It returns the first error encountered; per-schedule outcomes are logged only.
func EnsureSchedules(ctx context.Context, tc client.TemporalClient, cfg *config.Configuration, log *logger.Logger) error {
	for _, sc := range AllTemporalScheduleConfigs(cfg) {
		if !isScheduleEnabled(sc.ID, cfg) {
			log.Info(ctx, "schedule disabled by config, skipping", "id", sc.ID)
			continue
		}
		if err := ensureOneSchedule(ctx, tc, sc); err != nil {
			return err
		}
		log.Info(ctx, "schedule ensured", "id", sc.ID)
	}
	return nil
}

// isScheduleEnabled reports whether a schedule config should actually be created/updated
// against the Temporal server. Every schedule is enabled by default; add a case here only
// for a schedule that ships flag-gated off (see analytics.revenue_rollup.enabled).
func isScheduleEnabled(id types.ScheduleID, cfg *config.Configuration) bool {
	switch id {
	case types.ScheduleIDRevenueRollup:
		return cfg != nil && cfg.Analytics.RevenueRollup.Enabled
	default:
		return true
	}
}

func ensureOneSchedule(ctx context.Context, tc client.TemporalClient, cfg types.ScheduleConfig) error {
	id := string(cfg.ID)
	handle := tc.GetScheduleHandle(ctx, id)

	spec := sdkclient.ScheduleSpec{
		Intervals: []sdkclient.ScheduleIntervalSpec{
			{
				Every:  cfg.Interval,
				Offset: cfg.Offset,
			},
		},
	}

	_, err := handle.Describe(ctx)
	if err == nil {
		updateErr := handle.Update(ctx, sdkclient.ScheduleUpdateOptions{
			DoUpdate: func(in sdkclient.ScheduleUpdateInput) (*sdkclient.ScheduleUpdate, error) {
				in.Description.Schedule.Spec = &spec
				in.Description.Schedule.Action = &sdkclient.ScheduleWorkflowAction{
					Workflow:  cfg.Workflow,
					TaskQueue: cfg.TaskQueue.String(),
					Args:      []interface{}{cfg.Input},
				}
				in.Description.Schedule.Policy = &sdkclient.SchedulePolicies{
					Overlap: enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
				}
				return &sdkclient.ScheduleUpdate{Schedule: &in.Description.Schedule}, nil
			},
		})
		if updateErr != nil {
			return fmt.Errorf("update temporal schedule %q: %w", id, updateErr)
		}
		return nil
	}

	var notFound *serviceerror.NotFound
	if !errors.As(err, &notFound) {
		return fmt.Errorf("describe temporal schedule %q: %w", id, err)
	}

	_, createErr := tc.CreateSchedule(ctx, models.CreateScheduleOptions{
		ID:      id,
		Spec:    spec,
		Overlap: enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		Action: &sdkclient.ScheduleWorkflowAction{
			Workflow:  cfg.Workflow,
			TaskQueue: cfg.TaskQueue.String(),
			Args:      []interface{}{cfg.Input},
		},
	})
	if createErr != nil {
		return fmt.Errorf("create temporal schedule %q: %w", id, createErr)
	}
	return nil
}
