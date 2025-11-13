package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	temporalClient "github.com/flexprice/flexprice/internal/temporal/client"
	"github.com/flexprice/flexprice/internal/temporal/models"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"go.temporal.io/sdk/client"
)

const BillingActivityPrefix = "BillingActivities"

// SubscriptionBillingActivities contains all subscription billing-related activities
type SubscriptionBillingActivities struct {
	subscriptionRepo subscription.Repository
	invoiceRepo      invoice.Repository
	subscriptionSvc  service.SubscriptionService
	invoiceSvc       service.InvoiceService
	temporalClient   temporalClient.TemporalClient
	logger           *logger.Logger
}

// NewSubscriptionBillingActivities creates a new SubscriptionBillingActivities instance
func NewSubscriptionBillingActivities(
	subscriptionRepo subscription.Repository,
	invoiceRepo invoice.Repository,
	subscriptionSvc service.SubscriptionService,
	invoiceSvc service.InvoiceService,
	temporalClient temporalClient.TemporalClient,
	logger *logger.Logger,
) *SubscriptionBillingActivities {
	return &SubscriptionBillingActivities{
		subscriptionRepo: subscriptionRepo,
		invoiceRepo:      invoiceRepo,
		subscriptionSvc:  subscriptionSvc,
		invoiceSvc:       invoiceSvc,
		temporalClient:   temporalClient,
		logger:           logger,
	}
}

// FetchSubscriptionBatch fetches a batch of active subscription IDs
func (a *SubscriptionBillingActivities) FetchSubscriptionBatch(ctx context.Context, input models.FetchSubscriptionBatchInput) (*models.FetchSubscriptionBatchOutput, error) {
	// Validate input
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	a.logger.Infow("Fetching subscription batch",
		"batch_size", input.BatchSize,
		"offset", input.Offset,
		"tenant_id", input.TenantID,
		"environment_id", input.EnvironmentID)

	// Query active subscriptions
	batchSize := input.BatchSize
	offset := input.Offset
	filter := &types.SubscriptionFilter{
		QueryFilter: &types.QueryFilter{
			Limit:  &batchSize,
			Offset: &offset,
		},
		SubscriptionStatus: []types.SubscriptionStatus{types.SubscriptionStatusActive},
	}

	subscriptions, err := a.subscriptionRepo.List(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to fetch subscriptions").
			Mark(ierr.ErrDatabase)
	}

	// Extract IDs
	subscriptionIDs := make([]string, len(subscriptions))
	for i, sub := range subscriptions {
		subscriptionIDs[i] = sub.ID
	}

	a.logger.Infow("Fetched subscription batch",
		"count", len(subscriptionIDs),
		"batch_size", input.BatchSize,
		"offset", input.Offset)

	return &models.FetchSubscriptionBatchOutput{
		SubscriptionIDs: subscriptionIDs,
	}, nil
}

// EnqueueSubscriptionWorkflows starts independent workflows for each subscription
func (a *SubscriptionBillingActivities) EnqueueSubscriptionWorkflows(ctx context.Context, input models.EnqueueSubscriptionWorkflowsInput) error {
	// Validate input
	if err := input.Validate(); err != nil {
		return err
	}

	a.logger.Infow("Enqueueing subscription workflows",
		"count", len(input.SubscriptionIDs),
		"tenant_id", input.TenantID,
		"environment_id", input.EnvironmentID)

	successCount := 0
	failureCount := 0

	// Start workflow for each subscription
	for _, subscriptionID := range input.SubscriptionIDs {
		workflowInput := models.ProcessSingleSubscriptionWorkflowInput{
			SubscriptionID: subscriptionID,
			TenantID:       input.TenantID,
			EnvironmentID:  input.EnvironmentID,
		}

		workflowID := fmt.Sprintf("process-subscription-%s-%d", subscriptionID, time.Now().Unix())

		options := client.StartWorkflowOptions{
			ID:                       workflowID,
			TaskQueue:                types.TemporalTaskQueueBilling.String(),
			WorkflowExecutionTimeout: 30 * time.Minute,
		}

		_, err := a.temporalClient.GetRawClient().ExecuteWorkflow(ctx, options, types.TemporalProcessSingleSubscriptionWorkflow.String(), workflowInput)
		if err != nil {
			a.logger.Errorw("Failed to start workflow for subscription",
				"subscription_id", subscriptionID,
				"error", err)
			failureCount++
			// Continue with other subscriptions
			continue
		}

		successCount++
	}

	a.logger.Infow("Enqueued subscription workflows",
		"success", successCount,
		"failure", failureCount,
		"total", len(input.SubscriptionIDs))

	// Return error only if all failed
	if failureCount > 0 && successCount == 0 {
		return ierr.NewError("failed to enqueue any subscription workflows").
			WithHint("All subscription workflow enqueue attempts failed").
			Mark(ierr.ErrInternal)
	}

	return nil
}

// CheckPause checks if a subscription is paused
func (a *SubscriptionBillingActivities) CheckPause(ctx context.Context, input models.CheckPauseActivityInput) (*models.CheckPauseActivityOutput, error) {
	// Validate input
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	a.logger.Infow("Checking pause status", "subscription_id", input.SubscriptionID)

	// Get subscription
	sub, err := a.subscriptionRepo.Get(ctx, input.SubscriptionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to fetch subscription").
			Mark(ierr.ErrDatabase)
	}

	// Check if subscription is paused
	isPaused := sub.SubscriptionStatus == types.SubscriptionStatusPaused

	a.logger.Infow("Pause status checked",
		"subscription_id", input.SubscriptionID,
		"is_paused", isPaused)

	return &models.CheckPauseActivityOutput{
		IsPaused: isPaused,
	}, nil
}

// CalculateBillingPeriods calculates billing periods that need to be invoiced
func (a *SubscriptionBillingActivities) CalculateBillingPeriods(ctx context.Context, input models.CalculateBillingPeriodsActivityInput) (*models.CalculateBillingPeriodsActivityOutput, error) {
	// Validate input
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	a.logger.Infow("Calculating billing periods", "subscription_id", input.SubscriptionID)

	// Get subscription
	sub, err := a.subscriptionRepo.Get(ctx, input.SubscriptionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to fetch subscription").
			Mark(ierr.ErrDatabase)
	}

	// Calculate billing periods based on current period
	var periods []models.BillingPeriod

	currentStart := sub.CurrentPeriodStart
	currentEnd := sub.CurrentPeriodEnd
	now := time.Now()

	// If current period has ended, we need to bill for it
	if currentEnd.Before(now) || currentEnd.Equal(now) {
		period := models.BillingPeriod{
			SubscriptionID: input.SubscriptionID,
			StartDate:      currentStart,
			EndDate:        currentEnd,
			Amount:         decimal.Zero, // Will be calculated during invoice creation
		}
		periods = append(periods, period)

		a.logger.Infow("Found billing period to process",
			"subscription_id", input.SubscriptionID,
			"start_date", currentStart,
			"end_date", currentEnd)
	}

	a.logger.Infow("Calculated billing periods",
		"subscription_id", input.SubscriptionID,
		"periods_count", len(periods))

	return &models.CalculateBillingPeriodsActivityOutput{
		BillingPeriods: periods,
	}, nil
}

// CreateInvoice creates an invoice for a billing period
func (a *SubscriptionBillingActivities) CreateInvoice(ctx context.Context, input models.CreateInvoiceActivityInput) (*models.CreateInvoiceActivityOutput, error) {
	// Validate input
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	a.logger.Infow("Creating invoice for period",
		"subscription_id", input.SubscriptionID,
		"start_date", input.Period.StartDate,
		"end_date", input.Period.EndDate)

	// Check if invoice already exists for this subscription and period (idempotency)
	filter := &types.InvoiceFilter{
		SubscriptionID: input.SubscriptionID,
	}

	existingInvoices, err := a.invoiceRepo.List(ctx, filter)
	if err != nil {
		a.logger.Errorw("Failed to check existing invoices", "error", err)
		// Continue with invoice creation even if check fails
	} else {
		// Check if any existing invoice matches this period
		for _, inv := range existingInvoices {
			if inv.PeriodStart != nil && inv.PeriodEnd != nil {
				if inv.PeriodStart.Equal(input.Period.StartDate) && inv.PeriodEnd.Equal(input.Period.EndDate) {
					a.logger.Infow("Invoice already exists for this period",
						"invoice_id", inv.ID,
						"subscription_id", input.SubscriptionID)
					return &models.CreateInvoiceActivityOutput{
						InvoiceID: inv.ID,
					}, nil
				}
			}
		}
	}

	// Get subscription to get customer ID
	_, err = a.subscriptionRepo.Get(ctx, input.SubscriptionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to fetch subscription").
			Mark(ierr.ErrDatabase)
	}

	// Use subscription service to update billing periods
	// This will generate invoices automatically
	_, err = a.subscriptionSvc.UpdateBillingPeriods(ctx)
	if err != nil {
		a.logger.Errorw("Failed to update billing periods",
			"subscription_id", input.SubscriptionID,
			"error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to create invoice").
			Mark(ierr.ErrInternal)
	}

	// Get the newly created invoice
	invoices, err := a.invoiceRepo.List(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to fetch created invoice").
			Mark(ierr.ErrDatabase)
	}

	if len(invoices) == 0 {
		return nil, ierr.NewError("no invoice created").
			WithHint("Failed to create invoice for subscription").
			Mark(ierr.ErrInternal)
	}

	// Get the latest invoice
	latestInvoice := invoices[0]
	for _, inv := range invoices {
		if inv.CreatedAt.After(latestInvoice.CreatedAt) {
			latestInvoice = inv
		}
	}

	a.logger.Infow("Invoice created successfully",
		"invoice_id", latestInvoice.ID,
		"subscription_id", input.SubscriptionID)

	return &models.CreateInvoiceActivityOutput{
		InvoiceID: latestInvoice.ID,
	}, nil
}

// SyncToExternalVendor syncs invoice to external vendors (Stripe, Hubspot, etc.)
func (a *SubscriptionBillingActivities) SyncToExternalVendor(ctx context.Context, input models.SyncToExternalVendorActivityInput) error {
	// Validate input
	if err := input.Validate(); err != nil {
		return err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	a.logger.Infow("Syncing invoice to external vendors",
		"invoice_id", input.InvoiceID,
		"subscription_id", input.SubscriptionID)

	// Get invoice
	inv, err := a.invoiceRepo.Get(ctx, input.InvoiceID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to fetch invoice").
			Mark(ierr.ErrDatabase)
	}

	// TODO: Implement actual sync logic with external vendors
	// For now, we'll log and mark as successful
	// In production, this would call integration services like:
	// - Stripe: Create/update invoice
	// - HubSpot: Create/update deal/invoice
	// - Other payment providers

	a.logger.Infow("Invoice synced to external vendors (placeholder)",
		"invoice_id", input.InvoiceID,
		"subscription_id", input.SubscriptionID,
		"invoice_status", inv.InvoiceStatus)

	return nil
}

// AttemptPayment attempts to collect payment for an invoice
func (a *SubscriptionBillingActivities) AttemptPayment(ctx context.Context, input models.AttemptPaymentActivityInput) error {
	// Validate input
	if err := input.Validate(); err != nil {
		return err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	a.logger.Infow("Attempting payment for invoice",
		"invoice_id", input.InvoiceID,
		"subscription_id", input.SubscriptionID)

	// Get invoice
	inv, err := a.invoiceRepo.Get(ctx, input.InvoiceID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to fetch invoice").
			Mark(ierr.ErrDatabase)
	}

	// Check if payment already successful (idempotency)
	if inv.PaymentStatus == types.PaymentStatusSucceeded {
		a.logger.Infow("Invoice already paid", "invoice_id", input.InvoiceID)
		return nil
	}

	// Get subscription to check payment method
	sub, err := a.subscriptionRepo.Get(ctx, input.SubscriptionID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to fetch subscription").
			Mark(ierr.ErrDatabase)
	}

	// Check if subscription has payment method
	if sub.GatewayPaymentMethodID == nil || *sub.GatewayPaymentMethodID == "" {
		a.logger.Warnw("Subscription has no payment method",
			"subscription_id", input.SubscriptionID,
			"invoice_id", input.InvoiceID)
		return ierr.NewError("subscription has no payment method").
			WithHint("Cannot attempt payment without a payment method").
			Mark(ierr.ErrValidation)
	}

	// TODO: Implement actual payment logic
	// For now, we'll log and mark as attempted
	// In production, this would:
	// 1. Call payment provider API to charge payment method
	// 2. Update invoice payment status based on result
	// 3. Handle failures and retries appropriately

	a.logger.Infow("Payment attempt completed (placeholder)",
		"invoice_id", input.InvoiceID,
		"subscription_id", input.SubscriptionID,
		"payment_method_id", *sub.GatewayPaymentMethodID)

	return nil
}

// UpdateSubscriptionPeriod updates subscription metadata after billing periods are processed
func (a *SubscriptionBillingActivities) UpdateSubscriptionPeriod(ctx context.Context, input models.UpdateSubscriptionPeriodActivityInput) error {
	// Validate input
	if err := input.Validate(); err != nil {
		return err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	a.logger.Infow("Updating subscription period", "subscription_id", input.SubscriptionID)

	// Get subscription
	sub, err := a.subscriptionRepo.Get(ctx, input.SubscriptionID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to fetch subscription").
			Mark(ierr.ErrDatabase)
	}

	// Calculate next billing period
	nextPeriodStart := sub.CurrentPeriodEnd
	nextPeriodEnd := calculateNextPeriodEnd(nextPeriodStart, string(sub.BillingCadence))

	// Update subscription
	updateData := &subscription.Subscription{
		ID:                 sub.ID,
		CurrentPeriodStart: nextPeriodStart,
		CurrentPeriodEnd:   nextPeriodEnd,
	}

	err = a.subscriptionRepo.Update(ctx, updateData)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to update subscription period").
			Mark(ierr.ErrDatabase)
	}

	a.logger.Infow("Subscription period updated",
		"subscription_id", input.SubscriptionID,
		"next_period_start", nextPeriodStart,
		"next_period_end", nextPeriodEnd)

	return nil
}

// calculateNextPeriodEnd calculates the end date of the next billing period
func calculateNextPeriodEnd(start time.Time, cadence string) time.Time {
	switch cadence {
	case string(types.BILLING_PERIOD_MONTHLY):
		return start.AddDate(0, 1, 0)
	case string(types.BILLING_PERIOD_QUARTER):
		return start.AddDate(0, 3, 0)
	case string(types.BILLING_PERIOD_ANNUAL):
		return start.AddDate(1, 0, 0)
	default:
		return start.AddDate(0, 1, 0) // Default to monthly
	}
}
