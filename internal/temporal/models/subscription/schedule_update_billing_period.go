package subscription

import "github.com/flexprice/flexprice/internal/types"

type ScheduleSubscriptionUpdateBillingPeriodWorkflowInput struct {
	BatchSize int `json:"batch_size"`
}

// Validate validates the schedule subscription update billing period workflow input
func (i *ScheduleSubscriptionUpdateBillingPeriodWorkflowInput) Validate() error {
	if i.BatchSize <= 0 {
		i.BatchSize = types.DEFAULT_BATCH_SIZE
	}
	return nil
}

type ScheduleSubscriptionUpdateBillingPeriodWorkflowResult struct {
	SubscriptionIDs []string `json:"subscription_ids"`
}
