package models

import "time"

// RevenueRollupInput is the input for RevenueRollupWorkflow. Interval sizes the
// dirty-scan window (since = scheduled fire time - Interval); zero uses the
// workflow's default (1h). Since, when set, wins over the derived window — the
// handle for a manual/backfill run targeting a specific point in time (pass it
// in the workflow input; TemporalScheduledStartTime is a catch-up mechanism,
// not a business-window selector).
type RevenueRollupInput struct {
	Interval time.Duration `json:"interval"`
	Since    *time.Time    `json:"since,omitempty"`
}

// RevenueRollupWorkflowResult mirrors the counts from RevenueRollupService.RollupDirty.
type RevenueRollupWorkflowResult struct {
	Rolled  int `json:"rolled"`
	Skipped int `json:"skipped"`
}
