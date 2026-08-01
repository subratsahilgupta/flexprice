package models

import "time"

// ===================== MarketplaceUsageSnapshot (Cron A, every 6h) =====================

// MarketplaceUsageSnapshotWorkflowInput is the input for MarketplaceUsageSnapshotWorkflow.
// No fields required — period_start/period_end are derived inside the workflow from the run's
// scheduled_time (design doc FLE-981 §8.3).
type MarketplaceUsageSnapshotWorkflowInput struct{}

// MarketplaceUsageSnapshotActivityInput carries the reporting window computed by the workflow.
type MarketplaceUsageSnapshotActivityInput struct {
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
}

// MaxReportedIDs caps the ID lists carried on a workflow result. A result is persisted in Temporal's
// workflow history, so a tenant-wide failure (e.g. one broken connection) must not write an
// unbounded list into it. The counts are always exact; only the ID lists are truncated.
const MaxReportedIDs = 100

// MarketplaceUsageSnapshotWorkflowResult captures outcome metrics. Total counts distinct
// subscriptions walked; Succeeded counts those that now have a usage record for the window (whether
// this run wrote it or found one already there); Failed counts those that could not be snapshotted.
// The window is echoed back so a run's output is self-describing in the Temporal UI without having
// to cross-reference the schedule's fire time, and the ID lists name exactly which subscriptions
// landed and which need investigating, without grepping logs first.
type MarketplaceUsageSnapshotWorkflowResult struct {
	Total       int       `json:"total"`
	Succeeded   int       `json:"succeeded"`
	Failed      int       `json:"failed"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`

	// ID lists are truncated to MaxReportedIDs; the counts above carry the true totals.
	SucceededSubscriptionIDs []string `json:"succeeded_subscription_ids,omitempty"`
	FailedSubscriptionIDs    []string `json:"failed_subscription_ids,omitempty"`
}

// AppendSucceededSubscriptionID records a subscription that now has a usage record for the window.
func (r *MarketplaceUsageSnapshotWorkflowResult) AppendSucceededSubscriptionID(id string) {
	r.Succeeded++
	if len(r.SucceededSubscriptionIDs) < MaxReportedIDs {
		r.SucceededSubscriptionIDs = append(r.SucceededSubscriptionIDs, id)
	}
}

// AppendFailedSubscriptionID records a subscription that could not be snapshotted.
func (r *MarketplaceUsageSnapshotWorkflowResult) AppendFailedSubscriptionID(id string) {
	r.Failed++
	if len(r.FailedSubscriptionIDs) < MaxReportedIDs {
		r.FailedSubscriptionIDs = append(r.FailedSubscriptionIDs, id)
	}
}

// MarketplaceUsageReportWorkflowInput is the input for MarketplaceUsageReportWorkflow. It is
// empty; the activity reads all unsynced usage records itself.
type MarketplaceUsageReportWorkflowInput struct{}

// MarketplaceUsageReportWorkflowResult captures outcome metrics. Total counts records that reached
// at least one relevant connection this run; Succeeded counts those now fully synced to every
// relevant connection; Failed counts those still awaiting at least one. Skipped counts sync entries
// resolved without anything being posted (today only Azure's zero-quantity case), which would
// otherwise be indistinguishable from a real acceptance in Succeeded. The ID lists name the rows
// behind each count, so a run is actionable from the Temporal UI alone.
type MarketplaceUsageReportWorkflowResult struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`

	// ID lists are truncated to MaxReportedIDs; the counts above carry the true totals.
	SucceededRecordIDs []string `json:"succeeded_record_ids,omitempty"`
	FailedRecordIDs    []string `json:"failed_record_ids,omitempty"`
	SkippedRecordIDs   []string `json:"skipped_record_ids,omitempty"`
}

// AppendSucceededRecordID records a usage record now fully synced to every relevant connection.
func (r *MarketplaceUsageReportWorkflowResult) AppendSucceededRecordID(id string) {
	r.Succeeded++
	if len(r.SucceededRecordIDs) < MaxReportedIDs {
		r.SucceededRecordIDs = append(r.SucceededRecordIDs, id)
	}
}

// AppendFailedRecordID records a usage record that did not fully sync.
func (r *MarketplaceUsageReportWorkflowResult) AppendFailedRecordID(id string) {
	r.Failed++
	if len(r.FailedRecordIDs) < MaxReportedIDs {
		r.FailedRecordIDs = append(r.FailedRecordIDs, id)
	}
}

// AppendSkippedRecordID records a connection resolved without anything being posted. Skipped is
// counted per sync entry, so one record can appear here once per connection that skipped it.
func (r *MarketplaceUsageReportWorkflowResult) AppendSkippedRecordID(id string) {
	r.Skipped++
	if len(r.SkippedRecordIDs) < MaxReportedIDs {
		r.SkippedRecordIDs = append(r.SkippedRecordIDs, id)
	}
}

// ===================== MarketplaceSubscriptionFinalUsageFlush (triggered by CancelSubscription) =====================

// MarketplaceSubscriptionFinalUsageFlushWorkflowInput is the input for MarketplaceSubscriptionFinalUsageFlushWorkflow,
// started once per cancellation — not on a schedule. CancelAt must be the marketplace's own
// cancellation instant (sourced from sub.CancelAt, never sub.CancelledAt — see ERD FLE-1106 §3.8).
// TenantID/EnvironmentID are stamped on by temporalService.buildWorkflowInput from the triggering
// ctx — required so the workflow-tracking interceptor (which reads them via reflection off this
// struct) and every repository call inside the activity have a tenant to scope to.
type MarketplaceSubscriptionFinalUsageFlushWorkflowInput struct {
	SubscriptionID string    `json:"subscription_id"`
	CancelAt       time.Time `json:"cancel_at"`
	TenantID       string    `json:"tenant_id"`
	EnvironmentID  string    `json:"environment_id"`
}

// MarketplaceSubscriptionFinalUsageFlushActivityInput mirrors the workflow input; kept as a separate type so
// the activity signature doesn't change if the workflow input grows fields the activity doesn't need.
type MarketplaceSubscriptionFinalUsageFlushActivityInput struct {
	SubscriptionID string    `json:"subscription_id"`
	CancelAt       time.Time `json:"cancel_at"`
	TenantID       string    `json:"tenant_id"`
	EnvironmentID  string    `json:"environment_id"`
}

// MarketplaceSubscriptionFinalUsageFlushWorkflowResult captures what the flush actually did. Scoped to a single
// subscription, so it carries full IDs rather than the truncated lists the two scheduled results use —
// a flush covers at most a day's backlog across at most three marketplaces. The three record-ID lists
// mirror MarketplaceUsageReportWorkflowResult's classification exactly (both are produced by the same
// shared reportRecordToConnections), so the two report paths can never disagree on what "succeeded"
// means.
type MarketplaceSubscriptionFinalUsageFlushWorkflowResult struct {
	SubscriptionID string `json:"subscription_id"`

	// PeriodStart/PeriodEnd and FinalRecordID describe the final record this run wrote — PeriodEnd is
	// the true cancellation instant, without the reporting margin (that's applied on the wire only).
	// All three are zero/empty unless this run both computed AND fully reported the record. That
	// covers three different cases the same way: nothing was left to compute (cancel_at at or before
	// the frontier — also what a retry sees once an earlier attempt's record already advanced it), the
	// record's connection couldn't be resolved this run, or reporting it didn't fully succeed. The
	// last two also make this run return an error.
	PeriodStart   time.Time `json:"period_start,omitempty"`
	PeriodEnd     time.Time `json:"period_end,omitempty"`
	FinalRecordID string    `json:"final_record_id,omitempty"`

	// Record IDs cover the backlog plus the final record (once persisted, it's just another backlog
	// row), reported through the same shared loop.
	SucceededRecordIDs []string `json:"succeeded_record_ids,omitempty"`
	FailedRecordIDs    []string `json:"failed_record_ids,omitempty"`
	SkippedRecordIDs   []string `json:"skipped_record_ids,omitempty"`

	// DelinkedMappingIDs are the entity_integration_mapping rows archived by this run. Empty unless
	// every record above fully synced and every mapped connection could be resolved — a cancelled
	// subscription's mapping stays published on any failure so the next attempt (this activity's own
	// Temporal retry, or a future catch-up mechanism) can still find and retry it.
	DelinkedMappingIDs []string `json:"delinked_mapping_ids,omitempty"`
}
