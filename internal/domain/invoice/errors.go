package invoice

import "errors"

var (
	// ErrInvoiceNotFound indicates that the requested invoice was not found
	ErrInvoiceNotFound = errors.New("invoice not found")

	// ErrInvalidInvoiceStatus indicates that the invoice status transition is invalid
	ErrInvalidInvoiceStatus = errors.New("invalid invoice status")

	// ErrInvoiceAlreadyPaid indicates that the invoice has already been paid
	ErrInvoiceAlreadyPaid = errors.New("invoice already paid")

	// ErrInvoiceAlreadyVoided indicates that the invoice has already been voided
	ErrInvoiceAlreadyVoided = errors.New("invoice already voided")

	// ErrInvoiceNotFinalized indicates that the invoice is not in finalized status
	ErrInvoiceNotFinalized = errors.New("invoice not finalized")
)
