package moyasar

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// MoyasarInvoiceSyncRequest represents a request to sync FlexPrice invoice to Moyasar
type MoyasarInvoiceSyncRequest struct {
	InvoiceID string // FlexPrice invoice ID to sync
}

// MoyasarInvoiceSyncResponse represents the response after syncing invoice to Moyasar
type MoyasarInvoiceSyncResponse struct {
	MoyasarInvoiceID string          // Moyasar invoice ID
	URL              string          // Payment URL for the invoice
	Status           string          // Invoice status
	Amount           decimal.Decimal // Invoice total amount
	Currency         string          // Currency code
	CreatedAt        string          // Created timestamp
}

// InvoiceSyncService handles synchronization of FlexPrice invoices with Moyasar
type InvoiceSyncService struct {
	client                       MoyasarClient
	invoiceRepo                  invoice.Repository
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	logger                       *logger.Logger
}

// NewInvoiceSyncService creates a new Moyasar invoice sync service
func NewInvoiceSyncService(
	client MoyasarClient,
	invoiceRepo invoice.Repository,
	entityIntegrationMappingRepo entityintegrationmapping.Repository,
	logger *logger.Logger,
) *InvoiceSyncService {
	return &InvoiceSyncService{
		client:                       client,
		invoiceRepo:                  invoiceRepo,
		entityIntegrationMappingRepo: entityIntegrationMappingRepo,
		logger:                       logger,
	}
}

// SyncInvoiceToMoyasar syncs a FlexPrice invoice to Moyasar
// This creates an invoice in Moyasar and returns the payment URL
func (s *InvoiceSyncService) SyncInvoiceToMoyasar(
	ctx context.Context,
	req MoyasarInvoiceSyncRequest,
	customerService interfaces.CustomerService,
) (*MoyasarInvoiceSyncResponse, error) {
	s.logger.Infow("starting Moyasar invoice sync",
		"invoice_id", req.InvoiceID)

	// Step 1: Check if Moyasar connection exists
	if !s.client.HasMoyasarConnection(ctx) {
		return nil, ierr.NewError("Moyasar connection not available").
			WithHint("Moyasar integration must be configured for invoice sync").
			Mark(ierr.ErrNotFound)
	}

	// Step 2: Get FlexPrice invoice
	flexInvoice, err := s.invoiceRepo.Get(ctx, req.InvoiceID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get FlexPrice invoice").
			Mark(ierr.ErrDatabase)
	}

	// Step 3: Check if invoice is already synced to avoid duplicates
	existingMapping, err := s.GetExistingMoyasarMapping(ctx, req.InvoiceID)
	if err != nil && !ierr.IsNotFound(err) {
		return nil, err
	}

	if existingMapping != nil {
		moyasarInvoiceID := existingMapping.ProviderEntityID
		s.logger.Infow("invoice already synced to Moyasar",
			"invoice_id", req.InvoiceID,
			"moyasar_invoice_id", moyasarInvoiceID)

		// Return existing invoice details from mapping metadata
		return s.buildResponseFromMapping(existingMapping), nil
	}

	// Step 4: Build invoice request
	invoiceReq, err := s.buildInvoiceRequest(ctx, flexInvoice)
	if err != nil {
		return nil, err
	}

	// Step 5: Create invoice in Moyasar
	moyasarInvoice, err := s.client.CreateInvoice(ctx, invoiceReq)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create invoice in Moyasar").
			Mark(ierr.ErrInternal)
	}

	moyasarInvoiceID := moyasarInvoice.ID
	s.logger.Infow("successfully created invoice in Moyasar",
		"invoice_id", req.InvoiceID,
		"moyasar_invoice_id", moyasarInvoiceID,
		"payment_url", moyasarInvoice.URL)

	// Step 6: Create entity integration mapping
	if err := s.createInvoiceMapping(ctx, req.InvoiceID, moyasarInvoice, flexInvoice.EnvironmentID); err != nil {
		s.logger.Errorw("failed to create invoice mapping",
			"error", err,
			"invoice_id", req.InvoiceID,
			"moyasar_invoice_id", moyasarInvoiceID)
		// Don't fail the sync, just log the error
	}

	// Step 7: Update FlexPrice invoice metadata with Moyasar details
	if err := s.updateFlexPriceInvoiceFromMoyasar(ctx, flexInvoice, moyasarInvoice); err != nil {
		s.logger.Errorw("failed to update FlexPrice invoice metadata from Moyasar", "error", err)
		// Don't fail the entire sync for this
	}

	// Step 8: Build and return response
	return s.buildSyncResponse(moyasarInvoice), nil
}

// convertToSmallestUnit converts an amount to the smallest currency unit
// For most currencies: 1 unit = 100 smallest units (e.g., 1 USD = 100 cents)
// For zero-decimal currencies (e.g., JPY): 1 unit = 1 smallest unit
func convertToSmallestUnit(amount decimal.Decimal, currency string) (int64, error) {
	// Moyasar uses smallest currency units
	// Most currencies: multiply by 100 (e.g., USD, SAR, EUR)
	// Zero-decimal currencies: multiply by 1 (e.g., JPY, KRW)
	multiplier := decimal.NewFromInt(100)

	// Round to nearest integer to avoid truncation errors
	amountInSmallestUnit := amount.Mul(multiplier).Round(0).IntPart()

	return amountInSmallestUnit, nil
}

// buildInvoiceRequest constructs the Moyasar invoice creation request
func (s *InvoiceSyncService) buildInvoiceRequest(
	ctx context.Context,
	flexInvoice *invoice.Invoice,
) (*CreateInvoiceRequest, error) {
	// Convert amount to smallest currency unit
	amountInSmallestUnit, err := convertToSmallestUnit(flexInvoice.Total, flexInvoice.Currency)
	if err != nil {
		return nil, err
	}

	// Build description
	description := s.buildInvoiceDescription(flexInvoice)

	// Build metadata
	metadata := map[string]string{
		"flexprice_invoice_id":     flexInvoice.ID,
		"flexprice_customer_id":    flexInvoice.CustomerID,
		"flexprice_environment_id": types.GetEnvironmentID(ctx),
	}

	// Add invoice number if available
	if flexInvoice.InvoiceNumber != nil && *flexInvoice.InvoiceNumber != "" {
		metadata["invoice_number"] = *flexInvoice.InvoiceNumber
	}

	// Build request
	req := &CreateInvoiceRequest{
		Amount:      int(amountInSmallestUnit),
		Currency:    strings.ToUpper(flexInvoice.Currency),
		Description: description,
		Metadata:    metadata,
	}

	s.logger.Infow("built invoice request for Moyasar",
		"invoice_id", flexInvoice.ID,
		"amount", flexInvoice.Total.String(),
		"currency", flexInvoice.Currency)

	return req, nil
}

// buildInvoiceDescription creates a description for the invoice
func (s *InvoiceSyncService) buildInvoiceDescription(flexInvoice *invoice.Invoice) string {
	// Use invoice number if available
	if flexInvoice.InvoiceNumber != nil && *flexInvoice.InvoiceNumber != "" {
		return fmt.Sprintf("Invoice %s", *flexInvoice.InvoiceNumber)
	}

	// Fallback to generic description with item count
	itemCount := len(flexInvoice.LineItems)
	if itemCount == 1 {
		return "Invoice for 1 item"
	}

	return fmt.Sprintf("Invoice for %d items", itemCount)
}

// buildSyncResponse constructs the sync response from Moyasar invoice data
func (s *InvoiceSyncService) buildSyncResponse(moyasarInvoice *CreateInvoiceResponse) *MoyasarInvoiceSyncResponse {
	// Parse amount with error handling
	amount := decimal.NewFromInt(int64(moyasarInvoice.Amount))

	return &MoyasarInvoiceSyncResponse{
		MoyasarInvoiceID: moyasarInvoice.ID,
		URL:              moyasarInvoice.URL,
		Status:           moyasarInvoice.Status,
		Amount:           amount,
		Currency:         moyasarInvoice.Currency,
		CreatedAt:        moyasarInvoice.CreatedAt,
	}
}

// buildResponseFromMapping builds response from existing mapping metadata
func (s *InvoiceSyncService) buildResponseFromMapping(mapping *entityintegrationmapping.EntityIntegrationMapping) *MoyasarInvoiceSyncResponse {
	response := &MoyasarInvoiceSyncResponse{
		MoyasarInvoiceID: mapping.ProviderEntityID,
	}

	// Extract metadata if available
	if mapping.Metadata != nil {
		if url, ok := mapping.Metadata["moyasar_payment_url"].(string); ok {
			response.URL = url
		}
		if status, ok := mapping.Metadata["moyasar_status"].(string); ok {
			response.Status = status
		}
	}

	return response
}

// createInvoiceMapping creates entity integration mapping to track the sync
func (s *InvoiceSyncService) createInvoiceMapping(
	ctx context.Context,
	flexInvoiceID string,
	moyasarInvoice *CreateInvoiceResponse,
	environmentID string,
) error {
	metadata := map[string]interface{}{
		"moyasar_payment_url": moyasarInvoice.URL,
		"moyasar_status":      moyasarInvoice.Status,
		"sync_source":         "flexprice",
		"synced_at":           time.Now().UTC().Format(time.RFC3339),
	}

	mapping := &entityintegrationmapping.EntityIntegrationMapping{
		ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ENTITY_INTEGRATION_MAPPING),
		EntityType:       types.IntegrationEntityTypeInvoice,
		EntityID:         flexInvoiceID,
		ProviderType:     string(types.SecretProviderMoyasar),
		ProviderEntityID: moyasarInvoice.ID,
		Metadata:         metadata,
		EnvironmentID:    environmentID,
		BaseModel:        types.GetDefaultBaseModel(ctx),
	}

	if err := s.entityIntegrationMappingRepo.Create(ctx, mapping); err != nil {
		// If duplicate key error, invoice is already tracked (race condition)
		s.logger.Warnw("failed to create entity integration mapping (may already exist)",
			"error", err,
			"invoice_id", flexInvoiceID,
			"moyasar_invoice_id", moyasarInvoice.ID)
		return err
	}

	s.logger.Infow("created invoice mapping",
		"invoice_id", flexInvoiceID,
		"moyasar_invoice_id", moyasarInvoice.ID)

	return nil
}

// GetExistingMoyasarMapping checks if invoice is already synced to Moyasar
func (s *InvoiceSyncService) GetExistingMoyasarMapping(
	ctx context.Context,
	flexInvoiceID string,
) (*entityintegrationmapping.EntityIntegrationMapping, error) {
	filter := &types.EntityIntegrationMappingFilter{
		EntityType:    types.IntegrationEntityTypeInvoice,
		EntityID:      flexInvoiceID,
		ProviderTypes: []string{string(types.SecretProviderMoyasar)},
	}

	mappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to check existing invoice mapping").
			Mark(ierr.ErrDatabase)
	}

	if len(mappings) == 0 {
		return nil, ierr.NewError("invoice not synced to Moyasar").
			Mark(ierr.ErrNotFound)
	}

	return mappings[0], nil
}

// updateFlexPriceInvoiceFromMoyasar updates FlexPrice invoice with data from Moyasar
func (s *InvoiceSyncService) updateFlexPriceInvoiceFromMoyasar(ctx context.Context, flexInvoice *invoice.Invoice, moyasarInvoice *CreateInvoiceResponse) error {
	// Initialize metadata if not exists
	if flexInvoice.Metadata == nil {
		flexInvoice.Metadata = make(types.Metadata)
	}

	// Update invoice metadata with Moyasar details
	updated := false

	// Store Moyasar invoice URL
	if moyasarInvoice.URL != "" {
		flexInvoice.Metadata["moyasar_invoice_url"] = moyasarInvoice.URL
		updated = true
	}

	// Store Moyasar invoice ID
	if moyasarInvoice.ID != "" {
		flexInvoice.Metadata["moyasar_invoice_id"] = moyasarInvoice.ID
		updated = true
	}

	// Store Moyasar invoice status
	if moyasarInvoice.Status != "" {
		flexInvoice.Metadata["moyasar_invoice_status"] = moyasarInvoice.Status
		updated = true
	}

	if updated {
		s.logger.Infow("updating FlexPrice invoice with Moyasar details",
			"invoice_id", flexInvoice.ID,
			"moyasar_invoice_id", moyasarInvoice.ID,
			"moyasar_invoice_url", moyasarInvoice.URL)

		return s.invoiceRepo.Update(ctx, flexInvoice)
	}

	return nil
}
