package models

import "time"

// ===================== MarketplaceUsageSnapshot (Cron A, every 6h) =====================

// MarketplaceUsageSnapshotWorkflowInput is the input for the snapshot cron. It is empty: the
// reporting window is derived inside the workflow from the run's scheduled time.
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

// MarketplaceSubscriptionFinalUsageFlushWorkflowInput is the input for the cancellation flush.
//
// CancelAt must be the marketplace's own cancellation instant, which the tenant supplies on the
// cancellation request. It is not the time Flexprice processed that request: that is always later,
// and reporting against it would put the final usage after the cancellation the provider recorded.
//
// TenantID and EnvironmentID are stamped on from the triggering context when the workflow starts.
// They are required, not informational: the activity scopes every query by them, and workflow
// tracking reads them from this struct.
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

// MarketplaceSubscriptionFinalUsageFlushWorkflowResult captures what one flush did. It carries full
// id lists rather than truncated ones: a flush covers at most a day of records across at most three
// marketplaces.
type MarketplaceSubscriptionFinalUsageFlushWorkflowResult struct {
	SubscriptionID string `json:"subscription_id"`

	// The final usage record covering the span up to cancellation, whether this run created it or
	// found one an earlier attempt had already written. PeriodEnd is the true cancellation instant;
	// the reporting margin is applied only to the value sent to the providers. All three are empty
	// when no such record was needed, which is what a backdated cancellation looks like.
	PeriodStart   time.Time `json:"period_start,omitempty"`
	PeriodEnd     time.Time `json:"period_end,omitempty"`
	FinalRecordID string    `json:"final_record_id,omitempty"`

	// Every record this run reported, the backlog and the final one alike, split by outcome. A record
	// is succeeded once every marketplace mapped to it has accepted or skipped it and at least one
	// accepted; skipped when they all skipped; failed while any is still outstanding.
	SucceededRecordIDs []string `json:"succeeded_record_ids,omitempty"`
	FailedRecordIDs    []string `json:"failed_record_ids,omitempty"`
	SkippedRecordIDs   []string `json:"skipped_record_ids,omitempty"`

	// The marketplace mappings archived by this run. Empty unless every record above synced and every
	// mapped connection resolved: the mappings stay published on any failure so the reporting cron can
	// still find the outstanding records.
	DelinkedMappingIDs []string `json:"delinked_mapping_ids,omitempty"`
}
