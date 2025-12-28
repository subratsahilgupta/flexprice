package subscription

import (
	"context"

	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	subscriptionModels "github.com/flexprice/flexprice/internal/temporal/models/subscription"
	"github.com/flexprice/flexprice/internal/types"
)

type BillingActivities struct {
	subscriptionService service.SubscriptionService
	serviceParams       service.ServiceParams
	logger              *logger.Logger
}

func NewBillingActivities(
	subscriptionService service.SubscriptionService,
	serviceParams service.ServiceParams,
	logger *logger.Logger,
) *BillingActivities {
	return &BillingActivities{
		subscriptionService: subscriptionService,
		serviceParams:       serviceParams,
		logger:              logger,
	}
}

// CheckDraftSubscriptionActivity checks if the subscription is draft
func (s *BillingActivities) CheckDraftSubscriptionActivity(
	ctx context.Context,
	input subscriptionModels.CheckDraftSubscriptionAcitvityInput,
) (*subscriptionModels.CheckDraftSubscriptionAcitivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	sub, err := s.serviceParams.SubRepo.Get(ctx, input.SubscriptionID)
	if err != nil {
		return nil, err
	}

	if sub.SubscriptionStatus == types.SubscriptionStatusDraft {
		return &subscriptionModels.CheckDraftSubscriptionAcitivityOutput{
			IsDraft: true,
		}, nil
	}

	return &subscriptionModels.CheckDraftSubscriptionAcitivityOutput{
		IsDraft: false,
	}, nil
}

// CalculatePeriodsActivity calculates billing periods from the current period up to the specified time
func (s *BillingActivities) CalculatePeriodsActivity(
	ctx context.Context,
	input subscriptionModels.CalculatePeriodsActivityInput,
) (*subscriptionModels.CalculatePeriodsActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	subscriptionService := service.NewSubscriptionService(s.serviceParams)

	periods, err := subscriptionService.CalculateBillingPeriods(ctx, input.SubscriptionID)
	if err != nil {
		return nil, err
	}

	output := &subscriptionModels.CalculatePeriodsActivityOutput{
		Periods:       periods,
		ShouldProcess: len(periods) > 1,
	}

	return output, nil
}

// CreateInvoicesActivity creates and finalizes an invoice for a specific billing period
// This activity does NOT sync to external vendors or attempt payment - those are handled separately
func (s *BillingActivities) CreateInvoicesActivity(
	ctx context.Context,
	input subscriptionModels.CreateInvoicesActivityInput,
) (*subscriptionModels.CreateInvoicesActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	subscriptionService := service.NewSubscriptionService(s.serviceParams)
	invoiceService := service.NewInvoiceService(s.serviceParams)

	invoices := make([]string, 0)
	for _, period := range input.Periods {
		invoice, err := subscriptionService.CreateDraftInvoiceForSubscription(ctx, input.SubscriptionID, period)
		if err != nil {
			return nil, err
		}
		invoices = append(invoices, invoice.ID)
	}

	for _, invoice := range invoices {
		if err := invoiceService.FinalizeInvoice(ctx, invoice); err != nil {
			return nil, err
		}
	}

	return &subscriptionModels.CreateInvoicesActivityOutput{
		InvoiceIDs: invoices,
	}, nil
}

// UpdateCurrentPeriodActivity updates the subscription to the new current period
func (s *BillingActivities) UpdateCurrentPeriodActivity(
	ctx context.Context,
	input subscriptionModels.UpdateSubscriptionPeriodActivityInput,
) (*subscriptionModels.UpdateSubscriptionPeriodActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	// Get the subscription
	sub, err := s.serviceParams.SubRepo.Get(ctx, input.SubscriptionID)
	if err != nil {
		return nil, err
	}

	// Update to the new current period
	sub.CurrentPeriodStart = input.PeriodStart
	sub.CurrentPeriodEnd = input.PeriodEnd

	// Update the subscription
	if err := s.serviceParams.SubRepo.Update(ctx, sub); err != nil {
		s.logger.Errorw("failed to update subscription period",
			"subscription_id", sub.ID,
			"new_period_start", input.PeriodStart,
			"new_period_end", input.PeriodEnd,
			"error", err)
		return nil, err
	}

	s.logger.Infow("updated subscription period",
		"subscription_id", sub.ID,
		"new_period_start", input.PeriodStart,
		"new_period_end", input.PeriodEnd)

	return &subscriptionModels.UpdateSubscriptionPeriodActivityOutput{
		Success: true,
	}, nil
}

// SyncInvoiceActivity syncs an invoice to external vendors (Stripe, etc.)
func (s *BillingActivities) SyncInvoiceActivity(
	ctx context.Context,
	input subscriptionModels.SyncInvoiceToExternalVendorActivityInput,
) (*subscriptionModels.SyncInvoiceToExternalVendorActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	invoiceService := service.NewInvoiceService(s.serviceParams)

	for _, invoiceID := range input.InvoiceIDs {
		if err := invoiceService.SyncInvoiceToExternalVendors(ctx, invoiceID); err != nil {
			return nil, err
		}
	}

	return &subscriptionModels.SyncInvoiceToExternalVendorActivityOutput{
		Success: true,
	}, nil
}

// AttemptPaymentActivity attempts to collect payment for an invoice
func (s *BillingActivities) AttemptPaymentActivity(
	ctx context.Context,
	input subscriptionModels.AttemptPaymentActivityInput,
) (*subscriptionModels.AttemptPaymentActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	// Initialize invoice service
	invoiceService := service.NewInvoiceService(s.serviceParams)

	for _, invoiceID := range input.InvoiceIDs {
		if err := invoiceService.AttemptPayment(ctx, invoiceID); err != nil {
			return nil, err
		}
	}
	return &subscriptionModels.AttemptPaymentActivityOutput{
		Success: true,
	}, nil
}

// CheckCancellationActivity checks if a subscription should be cancelled and performs the cancellation
func (s *BillingActivities) CheckCancellationActivity(
	ctx context.Context,
	input subscriptionModels.CheckSubscriptionCancellationActivityInput,
) (*subscriptionModels.CheckSubscriptionCancellationActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	sub, err := s.serviceParams.SubRepo.Get(ctx, input.SubscriptionID)
	if err != nil {
		return nil, err
	}

	if sub.CancelAtPeriodEnd && sub.CancelAt != nil && !sub.CancelAt.After(input.Period.End) && sub.EndDate != nil && input.Period.End.Equal(*sub.EndDate) {
		sub.SubscriptionStatus = types.SubscriptionStatusCancelled
		sub.CancelledAt = sub.EndDate
		err := s.serviceParams.DB.WithTx(ctx, func(ctx context.Context) error {
			return s.serviceParams.SubRepo.Update(ctx, sub)
		})
		if err != nil {
			return nil, err
		}

	}
	return &subscriptionModels.CheckSubscriptionCancellationActivityOutput{
		Success: true,
	}, nil
}
