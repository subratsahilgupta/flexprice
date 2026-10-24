package telemetry

// trackedEventNames are the only event_name values ReportEvent sends.
// Every other webhook event is dropped before any payload work happens.
func trackedEventNames() map[string]bool {
	return map[string]bool{
		"customer.created":         true,
		"customer.deleted":         true,
		"subscription.created":     true,
		"subscription.activated":   true,
		"subscription.cancelled":   true,
		"credit_note.created":      true,
		"invoice.update.finalized": true,
		"payment.success":          true,
		"payment.failed":           true,
		"refund.succeeded":         true,
		"wallet.created":           true,
	}
}
