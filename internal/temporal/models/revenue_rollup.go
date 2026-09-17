package models

import "time"

// RevenueRollupInput is the input for RevenueRollupWorkflow. Interval sizes the
// dirty-scan window (since = scheduled fire time - Interval); zero uses the
// workflow's default (1h).
type RevenueRollupInput struct {
	Interval time.Duration `json:"interval"`
}

// RevenueRollupWorkflowResult mirrors the counts from RevenueRollupService.RollupDirty.
type RevenueRollupWorkflowResult struct {
	Rolled  int `json:"rolled"`
	Skipped int `json:"skipped"`
}
