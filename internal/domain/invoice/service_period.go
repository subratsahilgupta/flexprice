package invoice

import "time"

// ResolveServicePeriod returns the coverage envelope of this invoice's selected line items.
// A nil include selects all items. The end stays exclusive; invalid periods are ignored.
func (i *Invoice) ResolveServicePeriod(include func(*InvoiceLineItem) bool) (start, end *time.Time) {
	for _, item := range i.LineItems {
		if item == nil || item.PeriodStart == nil || item.PeriodEnd == nil {
			continue
		}
		if include != nil && !include(item) {
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
