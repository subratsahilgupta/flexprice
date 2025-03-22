package pdfgen

import "context"

type Repository interface {
	GetInvoiceDataWithLineItems(ctx context.Context, invoiceID string) (*InvoiceData, error)
}
