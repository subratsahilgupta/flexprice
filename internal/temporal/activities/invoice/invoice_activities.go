package invoice

import (
	"context"

	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	invoiceModels "github.com/flexprice/flexprice/internal/temporal/models/invoice"
	temporalService "github.com/flexprice/flexprice/internal/temporal/service"
	"github.com/flexprice/flexprice/internal/types"
)

const (
	// Workflow name for triggering invoice workflows
	WorkflowProcessInvoice = "ProcessInvoiceWorkflow"
)

// InvoiceActivities contains all invoice-related activities
type InvoiceActivities struct {
	serviceParams service.ServiceParams
	logger        *logger.Logger
}

// NewInvoiceActivities creates a new InvoiceActivities instance
func NewInvoiceActivities(
	serviceParams service.ServiceParams,
	logger *logger.Logger,
) *InvoiceActivities {
	return &InvoiceActivities{
		serviceParams: serviceParams,
		logger:        logger,
	}
}

// FinalizeInvoiceActivity finalizes an invoice
func (s *InvoiceActivities) FinalizeInvoiceActivity(
	ctx context.Context,
	input invoiceModels.FinalizeInvoiceActivityInput,
) (*invoiceModels.FinalizeInvoiceActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	invoiceService := service.NewInvoiceService(s.serviceParams)

	if err := invoiceService.FinalizeInvoice(ctx, input.InvoiceID); err != nil {
		s.logger.Errorw("failed to finalize invoice",
			"invoice_id", input.InvoiceID,
			"error", err)
		return nil, err
	}

	s.logger.Infow("finalized invoice successfully",
		"invoice_id", input.InvoiceID)

	return &invoiceModels.FinalizeInvoiceActivityOutput{
		Success: true,
	}, nil
}

// SyncInvoiceToVendorActivity syncs an invoice to external vendors
func (s *InvoiceActivities) SyncInvoiceToVendorActivity(
	ctx context.Context,
	input invoiceModels.SyncInvoiceActivityInput,
) (*invoiceModels.SyncInvoiceActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	invoiceService := service.NewInvoiceService(s.serviceParams)

	if err := invoiceService.SyncInvoiceToExternalVendors(ctx, input.InvoiceID); err != nil {
		s.logger.Errorw("failed to sync invoice to external vendor",
			"invoice_id", input.InvoiceID,
			"error", err)
		return nil, err
	}

	s.logger.Infow("synced invoice to external vendor successfully",
		"invoice_id", input.InvoiceID)

	return &invoiceModels.SyncInvoiceActivityOutput{
		Success: true,
	}, nil
}

// AttemptInvoicePaymentActivity attempts to collect payment for an invoice
func (s *InvoiceActivities) AttemptInvoicePaymentActivity(
	ctx context.Context,
	input invoiceModels.PaymentActivityInput,
) (*invoiceModels.PaymentActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	invoiceService := service.NewInvoiceService(s.serviceParams)

	if err := invoiceService.AttemptPayment(ctx, input.InvoiceID); err != nil {
		s.logger.Errorw("failed to attempt payment for invoice",
			"invoice_id", input.InvoiceID,
			"error", err)
		return nil, err
	}

	s.logger.Infow("attempted payment for invoice successfully",
		"invoice_id", input.InvoiceID)

	return &invoiceModels.PaymentActivityOutput{
		Success: true,
	}, nil
}

// TriggerInvoiceWorkflowActivity triggers invoice workflows for each invoice (fire-and-forget)
// If triggering fails for any invoice, it logs the error and continues with the rest
func (s *InvoiceActivities) TriggerInvoiceWorkflowActivity(
	ctx context.Context,
	input invoiceModels.TriggerInvoiceWorkflowActivityInput,
) (*invoiceModels.TriggerInvoiceWorkflowActivityOutput, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Set context values
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	temporalSvc := temporalService.GetGlobalTemporalService()

	output := &invoiceModels.TriggerInvoiceWorkflowActivityOutput{
		TriggeredCount: 0,
		FailedCount:    0,
		FailedInvoices: make([]string, 0),
	}

	for _, invoiceID := range input.InvoiceIDs {
		_, err := temporalSvc.ExecuteWorkflow(
			ctx,
			WorkflowProcessInvoice,
			invoiceModels.ProcessInvoiceWorkflowInput{
				InvoiceID:     invoiceID,
				TenantID:      input.TenantID,
				EnvironmentID: input.EnvironmentID,
			},
		)
		if err != nil {
			s.logger.Errorw("failed to trigger invoice workflow",
				"invoice_id", invoiceID,
				"error", err)
			output.FailedCount++
			output.FailedInvoices = append(output.FailedInvoices, invoiceID)
			// Continue with other invoices - don't fail the entire activity
			continue
		}

		s.logger.Infow("triggered invoice workflow",
			"invoice_id", invoiceID)
		output.TriggeredCount++
	}

	return output, nil
}
