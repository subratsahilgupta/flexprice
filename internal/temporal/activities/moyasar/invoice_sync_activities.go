package moyasar

import (
	"context"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/integration"
	"github.com/flexprice/flexprice/internal/integration/moyasar"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/temporal/models"
	"github.com/flexprice/flexprice/internal/types"
	"go.temporal.io/sdk/temporal"
)

// InvoiceSyncActivities handles Moyasar invoice sync activities
type InvoiceSyncActivities struct {
	integrationFactory *integration.Factory
	customerService    interfaces.CustomerService
	logger             *logger.Logger
}

// NewInvoiceSyncActivities creates a new Moyasar invoice sync activities handler
func NewInvoiceSyncActivities(
	integrationFactory *integration.Factory,
	customerService interfaces.CustomerService,
	logger *logger.Logger,
) *InvoiceSyncActivities {
	return &InvoiceSyncActivities{
		integrationFactory: integrationFactory,
		customerService:    customerService,
		logger:             logger,
	}
}

// SyncInvoiceToMoyasar syncs an invoice to Moyasar
// This is a thin wrapper around the Moyasar integration service
func (a *InvoiceSyncActivities) SyncInvoiceToMoyasar(
	ctx context.Context,
	input models.MoyasarInvoiceSyncWorkflowInput,
) error {
	a.logger.Infow("syncing invoice to Moyasar",
		"invoice_id", input.InvoiceID,
		"customer_id", input.CustomerID,
		"tenant_id", input.TenantID,
		"environment_id", input.EnvironmentID)

	// Set context values for tenant and environment
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	// Get Moyasar integration with runtime context
	moyasarIntegration, err := a.integrationFactory.GetMoyasarIntegration(ctx)
	if err != nil {
		if ierr.IsNotFound(err) {
			a.logger.Debugw("Moyasar connection not configured",
				"invoice_id", input.InvoiceID,
				"customer_id", input.CustomerID)
			// Return NON-RETRYABLE error - connection doesn't exist, retrying won't help
			return temporal.NewNonRetryableApplicationError(
				"Moyasar connection not configured",
				"ConnectionNotFound",
				err,
			)
		}
		a.logger.Errorw("failed to get Moyasar integration",
			"error", err,
			"invoice_id", input.InvoiceID,
			"customer_id", input.CustomerID)
		return err
	}

	// Perform the sync using the existing service
	syncReq := moyasar.MoyasarInvoiceSyncRequest{
		InvoiceID: input.InvoiceID,
	}

	_, err = moyasarIntegration.InvoiceSyncSvc.SyncInvoiceToMoyasar(ctx, syncReq, a.customerService)
	if err != nil {
		a.logger.Errorw("failed to sync invoice to Moyasar",
			"error", err,
			"invoice_id", input.InvoiceID)
		return err
	}

	a.logger.Infow("successfully synced invoice to Moyasar",
		"invoice_id", input.InvoiceID,
		"customer_id", input.CustomerID)

	return nil
}
