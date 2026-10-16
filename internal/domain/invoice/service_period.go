package invoice

import "time"

// ResolveServicePeriod returns the coverage envelope of the supplied line items.
// Callers select visible items; the end stays exclusive. Invalid periods are ignored.
func ResolveServicePeriod(items []*InvoiceLineItem) (start, end *time.Time) {
	for _, item := range items {
		if item == nil || item.PeriodStart == nil || item.PeriodEnd == nil {
			continue
		}
		if item.PeriodStart.IsZero() || item.PeriodEnd.IsZero() || item.PeriodEnd.Before(*item.PeriodStart) {
			continue
		}
		if start == nil || item.PeriodStart.Before(*start) {
			value := *item.PeriodStart
			start = &value
		}
		if end == nil || item.PeriodEnd.After(*end) {
			value := *item.PeriodEnd
			end = &value
		}
	}
	return start, end
}
