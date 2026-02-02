package searchattr

import (
	"fmt"

	"go.temporal.io/sdk/workflow"
)

const (
	// Custom Search Attribute Keys
	// These must be registered in your Temporal namespace before use
	SearchAttributeFailingActivity = "FailingActivity"
	SearchAttributeFailureReason   = "FailureReason"
	SearchAttributeSubscriptionID  = "SubscriptionID"
	SearchAttributeTenantID        = "TenantID"
	SearchAttributeEnvironmentID   = "EnvironmentID"
)

// UpsertFailureSearchAttributes upserts search attributes when an activity fails
// This allows querying failed workflows by activity name and failure reason in Temporal UI/CLI
//
// Parameters:
//   - ctx: Workflow context
//   - activityName: Name of the failed activity (e.g., "CreateDraftInvoicesActivity")
//   - err: The error that occurred
//   - subscriptionID: Optional subscription ID for additional filtering
//
// Example usage in workflow:
//
//	err := workflow.ExecuteActivity(ctx, ActivityCreateDraftInvoices, input).Get(ctx, &output)
//	if err != nil {
//	    UpsertFailureSearchAttributes(ctx, ActivityCreateDraftInvoices, err, input.SubscriptionID)
//	    return nil, err
//	}
func UpsertFailureSearchAttributes(
	ctx workflow.Context,
	activityName string,
	err error,
	subscriptionID string,
) {
	if err == nil {
		return
	}

	logger := workflow.GetLogger(ctx)

	// Build search attributes map
	searchAttrs := map[string]interface{}{
		SearchAttributeFailingActivity: activityName,
		SearchAttributeFailureReason:   truncateString(err.Error(), 2000), // Limit to 2000 chars
	}

	// Add subscription ID if provided
	if subscriptionID != "" {
		searchAttrs[SearchAttributeSubscriptionID] = subscriptionID
	}

	// Upsert the search attributes
	if upsertErr := workflow.UpsertSearchAttributes(ctx, searchAttrs); upsertErr != nil {
		logger.Error("Failed to upsert failure search attributes",
			"activity", activityName,
			"error", upsertErr)
	} else {
		logger.Info("Upserted failure search attributes",
			"activity", activityName,
			"failure_reason", truncateString(err.Error(), 200))
	}
}

// UpsertWorkflowSearchAttributes upserts general workflow search attributes
// Useful for setting searchable metadata at workflow start
//
// Example usage:
//
//	UpsertWorkflowSearchAttributes(ctx, map[string]interface{}{
//	    SearchAttributeSubscriptionID: input.SubscriptionID,
//	    SearchAttributeTenantID: input.TenantID,
//	    SearchAttributeEnvironmentID: input.EnvironmentID,
//	})
func UpsertWorkflowSearchAttributes(ctx workflow.Context, attrs map[string]interface{}) {
	logger := workflow.GetLogger(ctx)

	if err := workflow.UpsertSearchAttributes(ctx, attrs); err != nil {
		logger.Error("Failed to upsert workflow search attributes", "error", err)
	} else {
		logger.Debug("Upserted workflow search attributes", "count", len(attrs))
	}
}

// ClearFailureSearchAttributes clears failure-related search attributes
// Useful when retrying after a failure
func ClearFailureSearchAttributes(ctx workflow.Context) {
	logger := workflow.GetLogger(ctx)

	// Set empty strings to clear the attributes
	searchAttrs := map[string]interface{}{
		SearchAttributeFailingActivity: "",
		SearchAttributeFailureReason:   "",
	}

	if err := workflow.UpsertSearchAttributes(ctx, searchAttrs); err != nil {
		logger.Error("Failed to clear failure search attributes", "error", err)
	}
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GetSearchAttributeQueries returns example queries for filtering workflows
func GetSearchAttributeQueries() map[string]string {
	return map[string]string{
		"all_failed_workflows":      fmt.Sprintf("%s != ''", SearchAttributeFailingActivity),
		"specific_activity_failure": fmt.Sprintf("%s = 'CreateDraftInvoicesActivity'", SearchAttributeFailingActivity),
		"specific_error_message":    fmt.Sprintf("%s = 'quantity must be non-negative'", SearchAttributeFailureReason),
		"subscription_failures": fmt.Sprintf("%s != '' AND %s = 'subs_123'",
			SearchAttributeFailingActivity, SearchAttributeSubscriptionID),
		"tenant_failures": fmt.Sprintf("%s != '' AND %s = 'tenant_456'",
			SearchAttributeFailingActivity, SearchAttributeTenantID),
	}
}
