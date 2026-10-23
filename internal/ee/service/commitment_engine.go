package service

import (
	"sort"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/commitment"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// commitmentStack is a subscription's commitments and usage over a set of windows, ready to allocate.
type commitmentStack struct {
	Type        types.CommitmentType
	Measurement types.CommitmentMeasurement
	Windows     []time.Time
	Commitments []*stackedCommitment
	Lines       []*usageLine
}

// stackedCommitment is a commitment with its capacity laid out per window.
type stackedCommitment struct {
	C             *commitment.Commitment
	CommittedRate decimal.Decimal
	Period        commitmentPeriod
	Lines         []*usageLine // in fill order
	Pools         []capacityPool
	PoolOf        []int // window index to pool index, -1 when the commitment has no capacity in that window
}

// capacityPool is a capacity that one or more windows draw from.
type capacityPool struct {
	Capacity decimal.Decimal
}

// usageLine is one subscription line's usage, per window.
type usageLine struct {
	Item        *subscription.SubscriptionLineItem
	Price       *price.Price
	OverageRate *decimal.Decimal
	Quantity    decimal.Decimal
	Cost        decimal.Decimal
	Usage       []decimal.Decimal // quantity for quantity stacks, cost for amount stacks
}

// allocation is the result of filling a stack.
type allocation struct {
	Commitments []commitmentAllocation // aligned with stack.Commitments
	OnDemand    [][]decimal.Decimal    // [line][window], aligned with stack.Lines and stack.Windows
}

// commitmentAllocation is what one commitment absorbed.
type commitmentAllocation struct {
	Used       decimal.Decimal
	UsedByLine []decimal.Decimal // aligned with the commitment's Lines
	Capacity   decimal.Decimal
	Unused     decimal.Decimal
}

// sortCommitmentStack orders commitments by priority, then earlier start, then lower id.
func sortCommitmentStack(cs []*stackedCommitment) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i].C, cs[j].C
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}

		if !a.StartDate.Equal(b.StartDate) {
			return a.StartDate.Before(b.StartDate)
		}

		return a.ID < b.ID
	})
}

// allocate fills the stack as a waterfall: in each window every commitment is filled to capacity before the next.
func allocate(stack *commitmentStack) (*allocation, error) {
	windows := len(stack.Windows)

	onDemand := make([][]decimal.Decimal, len(stack.Lines))
	for i, line := range stack.Lines {
		if len(line.Usage) != windows {
			return nil, ierr.NewErrorf("usage line %d covers %d windows, stack has %d", i, len(line.Usage), windows).
				Mark(ierr.ErrInternal)
		}

		total := decimal.Zero
		for _, u := range line.Usage {
			total = total.Add(u)
		}

		want := line.Quantity
		if stack.Type == types.COMMITMENT_TYPE_AMOUNT {
			want = line.Cost
		}

		if !total.Equal(want) {
			return nil, ierr.NewErrorf("usage line %d windows sum to %s, expected %s", i, total, want).
				Mark(ierr.ErrInternal)
		}

		onDemand[i] = append([]decimal.Decimal(nil), line.Usage...)
	}

	out := &allocation{
		Commitments: make([]commitmentAllocation, len(stack.Commitments)),
		OnDemand:    onDemand,
	}

	remaining := make([][]decimal.Decimal, len(stack.Commitments))
	stackIndex := make([][]int, len(stack.Commitments))
	for ci, sc := range stack.Commitments {
		if len(sc.PoolOf) != windows {
			return nil, ierr.NewErrorf("commitment %s maps %d windows, stack has %d", sc.C.ID, len(sc.PoolOf), windows).
				Mark(ierr.ErrInternal)
		}

		for _, p := range sc.PoolOf {
			if p >= len(sc.Pools) {
				return nil, ierr.NewErrorf("commitment %s references pool %d of %d", sc.C.ID, p, len(sc.Pools)).
					Mark(ierr.ErrInternal)
			}
		}

		remaining[ci] = make([]decimal.Decimal, len(sc.Pools))
		capacity := decimal.Zero
		for pi, pool := range sc.Pools {
			remaining[ci][pi] = pool.Capacity
			capacity = capacity.Add(pool.Capacity)
		}

		stackIndex[ci] = make([]int, len(sc.Lines))
		for li, line := range sc.Lines {
			stackIndex[ci][li] = -1
			for si, candidate := range stack.Lines {
				if candidate == line {
					stackIndex[ci][li] = si
					break
				}
			}

			if stackIndex[ci][li] < 0 {
				return nil, ierr.NewErrorf("commitment %s fills a line that is not in the stack", sc.C.ID).
					Mark(ierr.ErrInternal)
			}
		}

		out.Commitments[ci] = commitmentAllocation{
			Used:       decimal.Zero,
			UsedByLine: make([]decimal.Decimal, len(sc.Lines)),
			Capacity:   capacity,
		}
	}

	for w := 0; w < windows; w++ {
		for ci, sc := range stack.Commitments {
			if sc.PoolOf[w] < 0 {
				continue
			}

			pool := &remaining[ci][sc.PoolOf[w]]
			for li := range sc.Lines {
				if !pool.IsPositive() {
					break
				}

				left := &onDemand[stackIndex[ci][li]][w]
				take := decimal.Min(*pool, *left)
				if !take.IsPositive() {
					continue
				}

				*pool = pool.Sub(take)
				*left = left.Sub(take)
				out.Commitments[ci].UsedByLine[li] = out.Commitments[ci].UsedByLine[li].Add(take)
				out.Commitments[ci].Used = out.Commitments[ci].Used.Add(take)
			}
		}
	}

	for ci := range out.Commitments {
		out.Commitments[ci].Unused = out.Commitments[ci].Capacity.Sub(out.Commitments[ci].Used)
	}

	return out, nil
}

// lineCost prices a quantity of usage on a price the way the normal billing path would.
type lineCost func(p *price.Price, qty decimal.Decimal) decimal.Decimal

// proportionalQuantity converts a money value on the line into units at its average rate.
func proportionalQuantity(line *usageLine, value decimal.Decimal) decimal.Decimal {
	if line.Cost.IsZero() {
		return decimal.Zero
	}

	return line.Quantity.Mul(value).Div(line.Cost)
}

func (l *usageLine) displayName() string {
	if l.Item.DisplayName != "" {
		return l.Item.DisplayName
	}

	return l.Price.DisplayName
}

// commitmentInvoiceLines renders each stack and its allocation (index-aligned) as within, true-up and on-demand lines.
func commitmentInvoiceLines(
	stacks []*commitmentStack,
	allocs []*allocation,
	currency string,
	periodStart, periodEnd time.Time,
	cost lineCost,
) []dto.CreateInvoiceLineItemRequest {
	newLine := func(
		c *commitment.Commitment,
		line *usageLine,
		kind types.CommitmentLineKind,
		name string,
		qty, amount decimal.Decimal,
	) dto.CreateInvoiceLineItemRequest {
		out := dto.CreateInvoiceLineItemRequest{
			DisplayName:  lo.ToPtr(name),
			Quantity:     qty,
			Amount:       types.RoundToCurrencyPrecision(amount, currency),
			PeriodStart:  lo.ToPtr(periodStart),
			PeriodEnd:    lo.ToPtr(periodEnd),
			CommitmentID: lo.ToPtr(c.ID),
			Metadata: types.Metadata{
				types.MetadataKeyCommitmentLineKind: string(kind),
				types.MetadataKeyCommitmentID:       c.ID,
			},
		}

		if c.InvoiceDescription != nil && kind != types.CommitmentLineOnDemand {
			out.Metadata[types.MetadataKeyDescription] = *c.InvoiceDescription
		}

		if line == nil {
			out.EntityType = lo.ToPtr(string(types.InvoiceLineItemEntityTypeCommitment))
			out.EntityID = lo.ToPtr(c.ID)
			return out
		}

		out.PriceID = lo.ToPtr(line.Item.PriceID)
		out.PriceType = lo.ToPtr(string(line.Item.PriceType))
		out.MeterID = lo.ToPtr(line.Item.MeterID)
		out.MeterDisplayName = lo.ToPtr(line.Item.MeterDisplayName)
		out.EntityID = lo.ToPtr(line.Item.EntityID)
		out.EntityType = lo.ToPtr(string(line.Item.EntityType))
		out.PlanDisplayName = lo.ToPtr(line.Item.PlanDisplayName)
		out.SubscriptionLineItemID = lo.ToPtr(line.Item.ID)
		return out
	}

	var out []dto.CreateInvoiceLineItemRequest
	for si, stack := range stacks {
		alloc := allocs[si]
		for ci, sc := range stack.Commitments {
			used := alloc.Commitments[ci]
			first := len(out)
			largest := -1
			usedValue, withinValue := decimal.Zero, decimal.Zero
			for li, line := range sc.Lines {
				if !used.UsedByLine[li].IsPositive() {
					continue
				}

				qty, amount := used.UsedByLine[li], used.UsedByLine[li].Mul(sc.CommittedRate)
				if stack.Type == types.COMMITMENT_TYPE_AMOUNT {
					qty, amount = proportionalQuantity(line, used.UsedByLine[li]), used.UsedByLine[li]
				}

				row := newLine(sc.C, line, types.CommitmentLineWithin, sc.C.InvoiceLabel, qty, amount)
				usedValue = usedValue.Add(amount)
				withinValue = withinValue.Add(row.Amount)
				if largest < 0 || row.Amount.GreaterThan(out[largest].Amount) {
					largest = len(out)
				}

				out = append(out, row)
			}

			// Lines rounded one by one can drift from their rounded total; the largest line absorbs it.
			if len(out) > first {
				drift := types.RoundToCurrencyPrecision(usedValue, currency).Sub(withinValue)
				out[largest].Amount = out[largest].Amount.Add(drift)
				withinValue = withinValue.Add(drift)
			}

			if !sc.C.Binding {
				continue
			}

			// True-up is the rounded period amount minus the rounded within lines, so they sum exactly.
			trueUp := types.RoundToCurrencyPrecision(sc.Period.Amount, currency).Sub(withinValue)
			if !trueUp.IsPositive() {
				continue
			}

			trueUpQty := used.Unused
			if stack.Type == types.COMMITMENT_TYPE_AMOUNT {
				trueUpQty = decimal.NewFromInt(1)
			}

			row := newLine(sc.C, nil, types.CommitmentLineTrueUp, sc.C.InvoiceLabel+" (unused)", trueUpQty, trueUp)
			row.PriceID = sc.C.CommittedPriceID
			row.PriceType = lo.ToPtr(string(types.PRICE_TYPE_FIXED))
			out = append(out, row)
		}

		for li, line := range stack.Lines {
			// The last commitment scoping the line carries its on-demand lines.
			owner := stack.Commitments[len(stack.Commitments)-1]
			for _, sc := range stack.Commitments {
				if lo.Contains(sc.Lines, line) {
					owner = sc
				}
			}

			// Window stacks price each window so bucketed prices match the normal path; the rest price the sum once.
			var qty, amount decimal.Decimal
			perWindow := stack.Measurement == types.CommitmentMeasurementWindow && line.OverageRate == nil
			for _, w := range alloc.OnDemand[li] {
				if !w.IsPositive() {
					continue
				}

				qty = qty.Add(w)
				switch {
				case stack.Type == types.COMMITMENT_TYPE_AMOUNT:
					amount = amount.Add(w)
				case perWindow:
					amount = amount.Add(cost(line.Price, w))
				}
			}

			if stack.Type == types.COMMITMENT_TYPE_QUANTITY && !perWindow && qty.IsPositive() {
				amount = cost(line.Price, qty)
				if line.OverageRate != nil {
					amount = qty.Mul(*line.OverageRate)
				}
			}

			switch {
			case qty.IsPositive() && stack.Type == types.COMMITMENT_TYPE_AMOUNT:
				out = append(out, newLine(owner.C, line, types.CommitmentLineOnDemand, line.displayName(), proportionalQuantity(line, qty), amount))
			case qty.IsPositive():
				out = append(out, newLine(owner.C, line, types.CommitmentLineOnDemand, line.displayName(), qty, amount))
			case stack.Type == types.COMMITMENT_TYPE_AMOUNT && line.Cost.IsZero() && line.Quantity.IsPositive():
				// Free usage takes no capacity, but its quantity still shows on the invoice.
				out = append(out, newLine(owner.C, line, types.CommitmentLineOnDemand, line.displayName(), line.Quantity, decimal.Zero))
			}
		}
	}

	return out
}
