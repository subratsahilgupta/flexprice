package subscription

const DEFAULT_BATCH_SIZE = 100

type ScheduleSubscriptionUpdateBillingPeriodWorkflowInput struct {
	BatchSize int `json:"batch_size"`
}

// Validate validates the schedule subscription update billing period workflow input
func (i *ScheduleSubscriptionUpdateBillingPeriodWorkflowInput) Validate() error {
	if i.BatchSize <= 0 {
		i.BatchSize = DEFAULT_BATCH_SIZE // Default to 100
	}
	return nil
}

type ScheduleSubscriptionUpdateBillingPeriodWorkflowResult struct {
	SubscriptionIDs []string `json:"subscription_ids"`
}
