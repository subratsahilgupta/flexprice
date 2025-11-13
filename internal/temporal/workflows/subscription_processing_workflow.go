package workflows

import (
	"time"

	"github.com/flexprice/flexprice/internal/temporal/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow name - must match the function name
	WorkflowProcessSingleSubscription = "ProcessSingleSubscriptionWorkflow"

	// Activity names for subscription processing
	ActivityCheckPause               = "CheckPause"
	ActivityCalculateBillingPeriods  = "CalculateBillingPeriods"
	ActivityCreateInvoice            = "CreateInvoice"
	ActivitySyncToExternalVendor     = "SyncToExternalVendor"
	ActivityAttemptPayment           = "AttemptPayment"
	ActivityUpdateSubscriptionPeriod = "UpdateSubscriptionPeriod"
)

// ProcessSingleSubscriptionWorkflow processes a single subscription independently
// It orchestrates all billing steps for the subscription
func ProcessSingleSubscriptionWorkflow(ctx workflow.Context, input models.ProcessSingleSubscriptionWorkflowInput) (*models.ProcessSingleSubscriptionWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)

	// Validate input
	if err := input.Validate(); err != nil {
		return nil, err
	}

	logger.Info("Starting subscription processing workflow", "subscription_id", input.SubscriptionID)

	// Define activity options with retries
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	result := &models.ProcessSingleSubscriptionWorkflowResult{
		SubscriptionID:          input.SubscriptionID,
		Status:                  "processing",
		BillingPeriodsProcessed: 0,
		InvoicesCreated:         []string{},
		CompletedAt:             workflow.Now(ctx),
	}

	// Step 1: Check if subscription is paused
	logger.Info("Checking if subscription is paused", "subscription_id", input.SubscriptionID)
	checkPauseInput := models.CheckPauseActivityInput{
		SubscriptionID: input.SubscriptionID,
		TenantID:       input.TenantID,
		EnvironmentID:  input.EnvironmentID,
	}

	var pauseOutput models.CheckPauseActivityOutput
	err := workflow.ExecuteActivity(ctx, ActivityCheckPause, checkPauseInput).Get(ctx, &pauseOutput)
	if err != nil {
		logger.Error("Failed to check pause status", "error", err, "subscription_id", input.SubscriptionID)
		errorMsg := err.Error()
		result.ErrorSummary = &errorMsg
		result.Status = "failed"
		return result, err
	}

	if pauseOutput.IsPaused {
		logger.Info("Subscription is paused, skipping billing", "subscription_id", input.SubscriptionID)
		result.Status = "skipped_paused"
		return result, nil
	}

	// Step 2: Calculate billing periods
	logger.Info("Calculating billing periods", "subscription_id", input.SubscriptionID)
	calculatePeriodsInput := models.CalculateBillingPeriodsActivityInput{
		SubscriptionID: input.SubscriptionID,
		TenantID:       input.TenantID,
		EnvironmentID:  input.EnvironmentID,
	}

	var periodsOutput models.CalculateBillingPeriodsActivityOutput
	err = workflow.ExecuteActivity(ctx, ActivityCalculateBillingPeriods, calculatePeriodsInput).Get(ctx, &periodsOutput)
	if err != nil {
		logger.Error("Failed to calculate billing periods", "error", err, "subscription_id", input.SubscriptionID)
		errorMsg := err.Error()
		result.ErrorSummary = &errorMsg
		result.Status = "failed"
		return result, err
	}

	if len(periodsOutput.BillingPeriods) == 0 {
		logger.Info("No billing periods to process", "subscription_id", input.SubscriptionID)
		result.Status = "completed_no_periods"
		return result, nil
	}

	logger.Info("Processing billing periods",
		"count", len(periodsOutput.BillingPeriods),
		"subscription_id", input.SubscriptionID)

	// Step 3: Process each billing period
	for i, period := range periodsOutput.BillingPeriods {
		logger.Info("Processing period",
			"subscription_id", input.SubscriptionID,
			"period_index", i,
			"start_date", period.StartDate,
			"end_date", period.EndDate)

		// Create invoice for this period
		createInvoiceInput := models.CreateInvoiceActivityInput{
			SubscriptionID: input.SubscriptionID,
			TenantID:       input.TenantID,
			EnvironmentID:  input.EnvironmentID,
			Period:         period,
		}

		var invoiceOutput models.CreateInvoiceActivityOutput
		err := workflow.ExecuteActivity(ctx, ActivityCreateInvoice, createInvoiceInput).Get(ctx, &invoiceOutput)
		if err != nil {
			logger.Error("Failed to create invoice",
				"error", err,
				"subscription_id", input.SubscriptionID,
				"period_index", i)
			continue // Continue with next period even if invoice creation fails
		}

		logger.Info("Invoice created",
			"invoice_id", invoiceOutput.InvoiceID,
			"subscription_id", input.SubscriptionID)

		result.InvoicesCreated = append(result.InvoicesCreated, invoiceOutput.InvoiceID)
		result.BillingPeriodsProcessed++

		// Sync to external vendors (Stripe, Hubspot, etc.)
		syncInput := models.SyncToExternalVendorActivityInput{
			InvoiceID:      invoiceOutput.InvoiceID,
			SubscriptionID: input.SubscriptionID,
			TenantID:       input.TenantID,
			EnvironmentID:  input.EnvironmentID,
		}

		// Use longer timeout for external sync
		syncCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 5 * time.Minute,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:    time.Second * 5,
				BackoffCoefficient: 2.0,
				MaximumInterval:    time.Minute * 5,
				MaximumAttempts:    5, // More tolerant for flaky external APIs
			},
		})

		err = workflow.ExecuteActivity(syncCtx, ActivitySyncToExternalVendor, syncInput).Get(syncCtx, nil)
		if err != nil {
			logger.Error("Failed to sync invoice to external vendors",
				"error", err,
				"invoice_id", invoiceOutput.InvoiceID,
				"subscription_id", input.SubscriptionID)
			// Continue even if sync fails
		} else {
			logger.Info("Invoice synced successfully",
				"invoice_id", invoiceOutput.InvoiceID,
				"subscription_id", input.SubscriptionID)
		}

		// Attempt payment
		paymentInput := models.AttemptPaymentActivityInput{
			InvoiceID:      invoiceOutput.InvoiceID,
			SubscriptionID: input.SubscriptionID,
			TenantID:       input.TenantID,
			EnvironmentID:  input.EnvironmentID,
		}

		err = workflow.ExecuteActivity(ctx, ActivityAttemptPayment, paymentInput).Get(ctx, nil)
		if err != nil {
			logger.Error("Failed to attempt payment",
				"error", err,
				"invoice_id", invoiceOutput.InvoiceID,
				"subscription_id", input.SubscriptionID)
			// Continue with next period; payment failures don't block other periods
		} else {
			logger.Info("Payment attempt successful",
				"invoice_id", invoiceOutput.InvoiceID,
				"subscription_id", input.SubscriptionID)
		}
	}

	// Step 4: Update subscription metadata after all periods processed
	logger.Info("Updating subscription period metadata", "subscription_id", input.SubscriptionID)
	updatePeriodInput := models.UpdateSubscriptionPeriodActivityInput{
		SubscriptionID: input.SubscriptionID,
		TenantID:       input.TenantID,
		EnvironmentID:  input.EnvironmentID,
	}

	err = workflow.ExecuteActivity(ctx, ActivityUpdateSubscriptionPeriod, updatePeriodInput).Get(ctx, nil)
	if err != nil {
		logger.Error("Failed to update subscription period metadata",
			"error", err,
			"subscription_id", input.SubscriptionID)
		errorMsg := err.Error()
		result.ErrorSummary = &errorMsg
		result.Status = "completed_with_errors"
		return result, err
	}

	result.Status = "completed"
	result.CompletedAt = workflow.Now(ctx)
	logger.Info("Successfully processed subscription",
		"subscription_id", input.SubscriptionID,
		"periods_processed", result.BillingPeriodsProcessed,
		"invoices_created", len(result.InvoicesCreated))

	return result, nil
}
