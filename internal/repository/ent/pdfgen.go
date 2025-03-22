package ent

import (
	"context"
	"strconv"
	"strings"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/customer"
	"github.com/flexprice/flexprice/ent/invoice"
	"github.com/flexprice/flexprice/ent/schema"
	"github.com/flexprice/flexprice/internal/domain/pdfgen"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
)

type invoicePdfGenRepository struct {
	client postgres.IClient
	log    *logger.Logger
}

func NewPdfGenRepository(client postgres.IClient, log *logger.Logger) pdfgen.Repository {
	return &invoicePdfGenRepository{
		client: client,
		log:    log,
	}
}

func (r *invoicePdfGenRepository) GetInvoiceDataWithLineItems(ctx context.Context, invoiceID string) (*pdfgen.InvoiceData, error) {
	client := r.client.Querier(ctx)
	// Query the invoice with line items
	inv, err := client.Invoice.Query().
		Where(invoice.ID(invoiceID)).
		WithLineItems().
		Only(ctx)
	if err != nil {
		return nil, ierr.WithError(err).WithHintf("failed to retrieve invoice %s", invoiceID).Mark(ierr.ErrDatabase)
	}

	// Fetch customer information using the CustomerID from the invoice
	customer, err := client.Customer.Query().
		Where(customer.ID(inv.CustomerID)).
		Only(ctx)
	if err != nil {
		return nil, ierr.WithError(err).WithHintf("failed to retrieve customer %s", inv.CustomerID).Mark(ierr.ErrDatabase)
	}

	// Get tenant-specific biller info
	billerInfo, err := r.getTenantBillerInfo(ctx, inv.TenantID)
	if err != nil {
		// Fall back to default biller info if tenant-specific info is not available
		billerInfo = &pdfgen.BillerInfo{}
	}

	invoiceNum := ""
	if inv.InvoiceNumber != nil {
		invoiceNum = *inv.InvoiceNumber
	}

	subID := ""
	if inv.SubscriptionID != nil {
		subID = *inv.SubscriptionID
	}

	// Convert to InvoiceData
	data := &pdfgen.InvoiceData{
		ID:              inv.ID,
		InvoiceNumber:   invoiceNum,
		CustomerID:      inv.CustomerID,
		SubscriptionID:  subID,
		InvoiceType:     inv.InvoiceType,
		InvoiceStatus:   inv.InvoiceStatus,
		PaymentStatus:   inv.PaymentStatus,
		Currency:        inv.Currency,
		AmountDue:       inv.AmountDue,
		AmountPaid:      inv.AmountPaid,
		AmountRemaining: inv.AmountRemaining,
		Description:     inv.Description,
		BillingReason:   inv.BillingReason,
		Notes:           "",   // Will be populated from metadata if available
		VAT:             0.18, // Default 18% VAT, could be from tenant config
		Biller:          billerInfo,
		Recipient:       extractRecipientInfo(customer),
	}

	// Convert dates
	if inv.DueDate != nil {
		data.DueDate = *inv.DueDate
	}
	if inv.PaidAt != nil {
		data.PaidAt = inv.PaidAt
	}
	if inv.VoidedAt != nil {
		data.VoidedAt = inv.VoidedAt
	}
	if inv.FinalizedAt != nil {
		data.FinalizedAt = inv.FinalizedAt
	}
	if inv.PeriodStart != nil {
		data.PeriodStart = inv.PeriodStart
	}
	if inv.PeriodEnd != nil {
		data.PeriodEnd = inv.PeriodEnd
	}

	// Parse metadata if available
	if inv.Metadata != nil {
		// Try to extract notes from metadata
		if notes, ok := inv.Metadata["notes"]; ok {
			data.Notes = notes
		}

		// Try to extract VAT from metadata
		if vat, ok := inv.Metadata["vat"]; ok {
			data.VAT, err = strconv.ParseFloat(vat, 64)
			if err != nil {
				return nil, ierr.WithError(err).WithHintf("failed to parse VAT %s", vat).Mark(ierr.ErrDatabase)
			}
		}
	}

	// Set default recipient if not found in metadata
	if data.Recipient.Name == "" {
		data.Recipient = defaultRecipientInfo(inv.CustomerID)
	}

	// Convert line items
	if len(inv.Edges.LineItems) > 0 {
		data.LineItems = make([]pdfgen.LineItemData, len(inv.Edges.LineItems))

		for i, item := range inv.Edges.LineItems {
			lineItem := pdfgen.LineItemData{
				PlanDisplayName: *item.PlanDisplayName,
				DisplayName:     *item.DisplayName,
				Amount:          item.Amount,
				Quantity:        item.Quantity,
				Currency:        item.Currency,
			}

			if item.PeriodStart != nil {
				lineItem.PeriodStart = item.PeriodStart
			}
			if item.PeriodEnd != nil {
				lineItem.PeriodEnd = item.PeriodEnd
			}

			data.LineItems[i] = lineItem
		}
	} else {
		return nil, ierr.NewError("no line items found").Mark(ierr.ErrDatabase)
	}

	return data, nil
}

func (r *invoicePdfGenRepository) getTenantBillerInfo(ctx context.Context, tenantID string) (*pdfgen.BillerInfo, error) {
	client := r.client.Querier(ctx)
	// Get tenant information
	tenant, err := client.Tenant.Get(ctx, tenantID)
	if err != nil {
		return nil, ierr.WithError(err).WithHintf("failed to retrieve tenant %s", tenantID).Mark(ierr.ErrDatabase)
	}

	// Extract billing info from the new field
	billerInfo := pdfgen.BillerInfo{
		Name: tenant.Name,
		Address: pdfgen.AddressInfo{
			Street:     "--",
			City:       "--",
			PostalCode: "--",
		},
	}

	// If billing_info is populated, use it to fill in the BillerInfo
	if tenant.BillingDetails != (schema.TenantBillingDetails{}) {
		billingDetails := tenant.BillingDetails
		billerInfo.Email = billingDetails.Email
		// billerInfo.Website = billingDetails.Website //TODO: Add this
		billerInfo.HelpEmail = billingDetails.HelpEmail
		// billerInfo.PaymentInstructions = billingDetails.PaymentInstructions //TODO: Add this
		billerInfo.Address = pdfgen.AddressInfo{
			Street:     strings.Join([]string{billingDetails.Address.Line1, billingDetails.Address.Line2}, "\n"),
			City:       billingDetails.Address.City,
			PostalCode: billingDetails.Address.PostalCode,
			Country:    billingDetails.Address.Country,
			State:      billingDetails.Address.State,
		}
	}

	return &billerInfo, nil
}

// defaultRecipientInfo returns default recipient information based on customer ID
func defaultRecipientInfo(customerID string) *pdfgen.RecipientInfo {
	return &pdfgen.RecipientInfo{
		Name: "Customer " + customerID,
		Address: pdfgen.AddressInfo{
			Street:     "--",
			City:       "--",
			PostalCode: "--",
		},
	}
}

// extractRecipientInfo extracts recipient information from metadata
func extractRecipientInfo(data *ent.Customer) *pdfgen.RecipientInfo {
	if data == nil {
		return &pdfgen.RecipientInfo{}
	}

	result := pdfgen.RecipientInfo{
		Name:  data.Name,
		Email: data.Email,
	}

	result.Address = pdfgen.AddressInfo{
		Street:     "--",
		City:       "--",
		PostalCode: "--",
	}

	if data.AddressLine1 != "" {
		result.Address.Street = data.AddressLine1
	}
	if data.AddressLine2 != "" {
		result.Address.Street += "\n" + data.AddressLine2
	}
	if data.AddressCity != "" {
		result.Address.City = data.AddressCity
	}
	if data.AddressState != "" {
		result.Address.State = data.AddressState
	}
	if data.AddressPostalCode != "" {
		result.Address.PostalCode = data.AddressPostalCode
	}
	if data.AddressCountry != "" {
		result.Address.Country = data.AddressCountry
	}

	return &result
}
