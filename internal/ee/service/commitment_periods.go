package service

import (
	"time"

	"github.com/flexprice/flexprice/internal/domain/commitment"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// commitmentPeriod is one subscription billing period clipped to the commitment term.
type commitmentPeriod struct {
	Start         time.Time
	End           time.Time
	FullStart     time.Time // the uncut billing period, used for proration
	FullEnd       time.Time
	ActiveBuckets int64 // window measurement only
	Capacity      decimal.Decimal
	Amount        decimal.Decimal // money value of Capacity
}

// commitmentPeriods returns the subscription billing periods clipped to the commitment term, with their value;
// amountOf turns a period's capacity into money.
func commitmentPeriods(
	c *commitment.Commitment,
	sub *subscription.Subscription,
	amountOf func(capacity decimal.Decimal) decimal.Decimal,
	bucket types.WindowSize,
) ([]commitmentPeriod, error) {
	full, err := types.CalculateBillingPeriods(&types.CalculateBillingPeriodsParams{
		InitialPeriodStart: sub.StartDate,
		EndDate:            &c.EndDate,
		Anchor:             sub.BillingAnchor,
		PeriodCount:        sub.BillingPeriodCount,
		BillingPeriod:      sub.BillingPeriod,
		Timezone:           sub.Timezone,
	})
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to compute billing periods for commitment").
			Mark(ierr.ErrInternal)
	}

	// Periods cut short by the subscription's start or end prorate against the regular period around them.
	grid, err := types.NewBillingPeriodGrid(sub.BillingAnchor, sub.BillingPeriod, sub.BillingPeriodCount, sub.Timezone)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to compute billing periods for commitment").
			Mark(ierr.ErrInternal)
	}

	out := make([]commitmentPeriod, 0, len(full))
	for _, p := range full {
		start, end := maxTime(p.Start, c.StartDate), minTime(p.End, c.EndDate)
		if !start.Before(end) {
			continue
		}

		regular := types.FullBillingPeriod(p.Start, grid)
		cp := commitmentPeriod{
			Start:     start,
			End:       end,
			FullStart: regular.Start,
			FullEnd:   regular.End,
		}
		fillPeriodValue(c, &cp, amountOf, bucket, sub.BillingAnchor)
		out = append(out, cp)
	}

	return out, nil
}

// clipCommitmentPeriod returns p cut to [from, until), with its capacity and amount recomputed.
// Bounds within the same second as p's edges leave it whole, since computed billing dates drop sub-seconds.
func clipCommitmentPeriod(
	c *commitment.Commitment,
	p commitmentPeriod,
	from, until time.Time,
	amountOf func(capacity decimal.Decimal) decimal.Decimal,
	bucket types.WindowSize,
	anchor time.Time,
) commitmentPeriod {
	out := p
	if from.Truncate(time.Second).After(p.Start.Truncate(time.Second)) {
		out.Start = minTime(from, p.End)
	}

	if until.Truncate(time.Second).Before(p.End.Truncate(time.Second)) {
		out.End = maxTime(until, out.Start)
	}

	if out.Start.Equal(p.Start) && out.End.Equal(p.End) {
		return p
	}

	fillPeriodValue(c, &out, amountOf, bucket, anchor)
	return out
}

// fillPeriodValue sets the capacity and money value of cp from its Start and End.
func fillPeriodValue(
	c *commitment.Commitment,
	cp *commitmentPeriod,
	amountOf func(capacity decimal.Decimal) decimal.Decimal,
	bucket types.WindowSize,
	anchor time.Time,
) {
	cp.ActiveBuckets = 0
	if c.Measurement == types.CommitmentMeasurementWindow {
		// A bucket starting before the period belongs to the period that already counted it.
		first := truncateToBucketStart(cp.Start, bucket, &anchor)
		if first.Before(cp.Start) {
			first = nextBucketStart(first, bucket, &anchor)
		}

		if dur, fixed := bucketDuration(bucket); fixed && len(c.ActiveWindows) == 0 {
			// Clock-aligned buckets with no active windows count arithmetically: a minute-bucket
			// year would otherwise walk half a million instants on every value computation.
			if first.Before(cp.End) {
				cp.ActiveBuckets = int64(cp.End.Sub(first)-1)/int64(dur) + 1
			}
		} else {
			loc := c.Location()
			for b := first; b.Before(cp.End); b = nextBucketStart(b, bucket, &anchor) {
				if c.ActiveWindows.Contains(b, loc) {
					cp.ActiveBuckets++
				}
			}
		}

		cp.Capacity = c.Value.Mul(decimal.NewFromInt(cp.ActiveBuckets))
	} else {
		fraction := decimal.NewFromInt(int64(cp.End.Sub(cp.Start))).Div(decimal.NewFromInt(int64(cp.FullEnd.Sub(cp.FullStart))))
		cp.Capacity = c.Value.Mul(fraction)
	}

	cp.Amount = amountOf(cp.Capacity)
}

// commitmentValue sums the money value of all periods.
func commitmentValue(periods []commitmentPeriod) decimal.Decimal {
	total := decimal.Zero
	for _, p := range periods {
		total = total.Add(p.Amount)
	}

	return total
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}

	return b
}

// bucketDuration returns the fixed step of clock-aligned bucket sizes; calendar sizes (day, week,
// month) step by calendar rules and return false.
func bucketDuration(size types.WindowSize) (time.Duration, bool) {
	switch size {
	case types.WindowSizeMinute:
		return time.Minute, true
	case types.WindowSize15Min:
		return 15 * time.Minute, true
	case types.WindowSize30Min:
		return 30 * time.Minute, true
	case types.WindowSizeHour:
		return time.Hour, true
	case types.WindowSize3Hour:
		return 3 * time.Hour, true
	case types.WindowSize6Hour:
		return 6 * time.Hour, true
	case types.WindowSize12Hour:
		return 12 * time.Hour, true
	default:
		return 0, false
	}
}
