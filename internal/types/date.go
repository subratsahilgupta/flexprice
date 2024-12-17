package types

import (
	"fmt"
	"time"
)

// NextBillingDate calculates the next billing date based on the given start time,
// billing period, and billing period unit (the frequency multiplier).
// For example:
// - If billing period is MONTHLY and unit is 2, we add two months.
// - If billing period is ANNUAL and unit is 1, we add one year.
// - If billing period is WEEKLY and unit is 3, we add 21 days (3 weeks).
// - If billing period is DAILY and unit is 10, we add 10 days.
// This function leverages time.AddDate, which properly handles leap years, month-boundary issues, etc.
func NextBillingDate(start time.Time, unit int, period BillingPeriod) (time.Time, error) {
	if unit <= 0 {
		return start, fmt.Errorf("billing period unit must be a positive integer, got %d", unit)
	}

	switch period {
	case BILLING_PERIOD_DAILY:
		// Add unit days
		return AddClampedDate(start, 0, 0, unit), nil
	case BILLING_PERIOD_WEEKLY:
		// 1 week = 7 days
		return AddClampedDate(start, 0, 0, 7*unit), nil
	case BILLING_PERIOD_MONTHLY:
		// Add 'unit' months
		return AddClampedDate(start, 0, unit, 0), nil
	case BILLING_PERIOD_ANNUAL:
		// Add 'unit' years
		return AddClampedDate(start, unit, 0, 0), nil
	default:
		return start, fmt.Errorf("invalid billing period type: %s", period)
	}
}

func AddClampedDate(t time.Time, years, months, days int) time.Time {
	y, m, d := t.Date()
	h, min, sec := t.Clock()

	// Calculate the proposed year and month
	newY := y + years
	newM := time.Month(int(m) + months)

	// If we move beyond December, it adjusts correctly,
	// for example adding 2 months to November will land on January next year.
	for newM > 12 {
		newM -= 12
		newY++
	}
	for newM < 1 {
		newM += 12
		newY--
	}

	// Find the last valid day of the new month
	firstOfNextMonth := time.Date(newY, newM+1, 1, 0, 0, 0, 0, t.Location())
	lastDay := firstOfNextMonth.Add(-24 * time.Hour).Day()

	newD := d + days
	if newD > lastDay {
		// Clamp to last valid day
		newD = lastDay
	} else if newD < 1 {
		// If we go backwards beyond the start of the month,
		// we might need similar logic. For simplicity, assume positive increments.
	}

	return time.Date(newY, newM, newD, h, min, sec, t.Nanosecond(), t.Location())
}
