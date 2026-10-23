package service

import (
	"sort"
	"time"

	"github.com/flexprice/flexprice/internal/domain/commitment"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
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
