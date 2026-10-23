package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/commitment"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func intDec(v int64) decimal.Decimal { return decimal.NewFromInt(v) }

func intDecs(vs ...int64) []decimal.Decimal {
	out := make([]decimal.Decimal, len(vs))
	for i, v := range vs {
		out[i] = intDec(v)
	}

	return out
}

func engineWindows(n int) []time.Time {
	start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]time.Time, n)
	for i := range out {
		out[i] = start.AddDate(0, 0, i)
	}

	return out
}

func quantityLine(usage ...int64) *usageLine {
	total := int64(0)
	for _, u := range usage {
		total += u
	}

	return &usageLine{Quantity: intDec(total), Usage: intDecs(usage...)}
}

// stacked builds a commitment with the given pool capacities and window-to-pool mapping.
func stacked(id string, priority int, poolOf []int, lines []*usageLine, capacities ...int64) *stackedCommitment {
	pools := make([]capacityPool, len(capacities))
	for i, c := range capacities {
		pools[i] = capacityPool{Capacity: intDec(c)}
	}

	return &stackedCommitment{
		C:      &commitment.Commitment{ID: id, Priority: priority},
		Lines:  lines,
		Pools:  pools,
		PoolOf: poolOf,
	}
}

func requireDecimals(t *testing.T, want []int64, got []decimal.Decimal) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		require.True(t, intDec(want[i]).Equal(got[i]), "index %d: want %d, got %s", i, want[i], got[i])
	}
}

func TestAllocate(t *testing.T) {
	tests := []struct {
		name         string
		build        func() *commitmentStack
		wantUsed     []int64
		wantUnused   []int64
		wantCapacity []int64
		wantOnDemand [][]int64
	}{
		{
			name: "E17 usage below first commitment",
			build: func() *commitmentStack {
				return e17Stack(6)
			},
			wantUsed:     []int64{6, 0},
			wantUnused:   []int64{4, 10},
			wantCapacity: []int64{10, 10},
			wantOnDemand: [][]int64{{0}},
		},
		{
			name: "E17 usage spills into second commitment",
			build: func() *commitmentStack {
				return e17Stack(15)
			},
			wantUsed:     []int64{10, 5},
			wantUnused:   []int64{0, 5},
			wantCapacity: []int64{10, 10},
			wantOnDemand: [][]int64{{0}},
		},
		{
			name: "E17 usage exceeds the whole stack",
			build: func() *commitmentStack {
				return e17Stack(24)
			},
			wantUsed:     []int64{10, 10},
			wantUnused:   []int64{0, 0},
			wantCapacity: []int64{10, 10},
			wantOnDemand: [][]int64{{4}},
		},
		{
			name: "E18 second commitment outside its window sends overflow on demand",
			build: func() *commitmentStack {
				line := quantityLine(15, 15)
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_QUANTITY,
					Measurement: types.CommitmentMeasurementWindow,
					Windows:     engineWindows(2),
					Lines:       []*usageLine{line},
					Commitments: []*stackedCommitment{
						stacked("c1", 1, []int{0, 1}, []*usageLine{line}, 10, 10),
						stacked("c2", 2, []int{-1, 0}, []*usageLine{line}, 10),
					},
				}
			},
			wantUsed:     []int64{20, 5},
			wantUnused:   []int64{0, 5},
			wantCapacity: []int64{20, 10},
			wantOnDemand: [][]int64{{5, 0}},
		},
		{
			name: "E3 quantity per window with on demand",
			build: func() *commitmentStack {
				line := quantityLine(80, 150, 100)
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_QUANTITY,
					Measurement: types.CommitmentMeasurementWindow,
					Windows:     engineWindows(3),
					Lines:       []*usageLine{line},
					Commitments: []*stackedCommitment{
						stacked("c1", 1, []int{0, 1, 2}, []*usageLine{line}, 100, 100, 100),
					},
				}
			},
			wantUsed:     []int64{280},
			wantUnused:   []int64{20},
			wantCapacity: []int64{300},
			wantOnDemand: [][]int64{{0, 50, 0}},
		},
		{
			name: "billing period pool drains across stretches and gives nothing before its start",
			build: func() *commitmentStack {
				line := quantityLine(30, 60, 60, 20, 30)
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_QUANTITY,
					Measurement: types.CommitmentMeasurementBillingPeriod,
					Windows:     engineWindows(5),
					Lines:       []*usageLine{line},
					Commitments: []*stackedCommitment{
						stacked("c1", 1, []int{-1, 0, 0, -1, 0}, []*usageLine{line}, 100),
					},
				}
			},
			wantUsed:     []int64{100},
			wantUnused:   []int64{0},
			wantCapacity: []int64{100},
			wantOnDemand: [][]int64{{30, 0, 20, 20, 30}},
		},
		{
			name: "E8 meter scope lines share one capacity in fill order",
			build: func() *commitmentStack {
				a, b := quantityLine(70), quantityLine(50)
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_QUANTITY,
					Measurement: types.CommitmentMeasurementBillingPeriod,
					Windows:     engineWindows(1),
					Lines:       []*usageLine{a, b},
					Commitments: []*stackedCommitment{
						stacked("c1", 1, []int{0}, []*usageLine{a, b}, 100),
					},
				}
			},
			wantUsed:     []int64{100},
			wantUnused:   []int64{0},
			wantCapacity: []int64{100},
			wantOnDemand: [][]int64{{0}, {20}},
		},
		{
			name: "E8 fill order follows the commitment's line order not the stack's",
			build: func() *commitmentStack {
				a, b := quantityLine(70), quantityLine(50)
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_QUANTITY,
					Measurement: types.CommitmentMeasurementBillingPeriod,
					Windows:     engineWindows(1),
					Lines:       []*usageLine{a, b},
					Commitments: []*stackedCommitment{
						stacked("c1", 1, []int{0}, []*usageLine{b, a}, 100),
					},
				}
			},
			wantUsed:     []int64{100},
			wantUnused:   []int64{0},
			wantCapacity: []int64{100},
			wantOnDemand: [][]int64{{20}, {0}},
		},
		{
			name: "amount stack allocates cost",
			build: func() *commitmentStack {
				line := &usageLine{Cost: intDec(130), Usage: intDecs(50, 80)}
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_AMOUNT,
					Measurement: types.CommitmentMeasurementWindow,
					Windows:     engineWindows(2),
					Lines:       []*usageLine{line},
					Commitments: []*stackedCommitment{
						stacked("c1", 1, []int{0, 1}, []*usageLine{line}, 60, 60),
					},
				}
			},
			wantUsed:     []int64{110},
			wantUnused:   []int64{10},
			wantCapacity: []int64{120},
			wantOnDemand: [][]int64{{0, 20}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := allocate(tt.build())
			require.NoError(t, err)
			require.Len(t, got.Commitments, len(tt.wantUsed))
			for i := range tt.wantUsed {
				require.True(t, intDec(tt.wantUsed[i]).Equal(got.Commitments[i].Used), "commitment %d used: %s", i, got.Commitments[i].Used)
				require.True(t, intDec(tt.wantUnused[i]).Equal(got.Commitments[i].Unused), "commitment %d unused: %s", i, got.Commitments[i].Unused)
				require.True(t, intDec(tt.wantCapacity[i]).Equal(got.Commitments[i].Capacity), "commitment %d capacity: %s", i, got.Commitments[i].Capacity)
			}

			require.Len(t, got.OnDemand, len(tt.wantOnDemand))
			for i := range tt.wantOnDemand {
				requireDecimals(t, tt.wantOnDemand[i], got.OnDemand[i])
			}
		})
	}
}

func e17Stack(usage int64) *commitmentStack {
	line := quantityLine(usage)
	return &commitmentStack{
		Type:        types.COMMITMENT_TYPE_QUANTITY,
		Measurement: types.CommitmentMeasurementBillingPeriod,
		Windows:     engineWindows(1),
		Lines:       []*usageLine{line},
		Commitments: []*stackedCommitment{
			stacked("c1", 1, []int{0}, []*usageLine{line}, 10),
			stacked("c2", 2, []int{0}, []*usageLine{line}, 10),
		},
	}
}

func TestAllocateUsedByLine(t *testing.T) {
	a, b := quantityLine(70, 0), quantityLine(10, 50)
	stack := &commitmentStack{
		Type:        types.COMMITMENT_TYPE_QUANTITY,
		Measurement: types.CommitmentMeasurementWindow,
		Windows:     engineWindows(2),
		Lines:       []*usageLine{a, b},
		Commitments: []*stackedCommitment{
			stacked("c1", 1, []int{0, 1}, []*usageLine{b, a}, 40, 40),
		},
	}

	got, err := allocate(stack)
	require.NoError(t, err)

	// UsedByLine follows the commitment's own Lines order: b then a.
	requireDecimals(t, []int64{50, 30}, got.Commitments[0].UsedByLine)
	require.True(t, intDec(80).Equal(got.Commitments[0].Used))
}

func TestAllocateRejectsBrokenInput(t *testing.T) {
	tests := []struct {
		name  string
		build func() *commitmentStack
	}{
		{
			name: "quantity not conserved",
			build: func() *commitmentStack {
				line := &usageLine{Quantity: intDec(100), Usage: intDecs(40, 50)}
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_QUANTITY,
					Windows:     engineWindows(2),
					Lines:       []*usageLine{line},
					Commitments: []*stackedCommitment{stacked("c1", 1, []int{0, 0}, []*usageLine{line}, 100)},
				}
			},
		},
		{
			name: "cost not conserved on amount stack",
			build: func() *commitmentStack {
				line := &usageLine{Quantity: intDec(90), Cost: intDec(100), Usage: intDecs(40, 50)}
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_AMOUNT,
					Windows:     engineWindows(2),
					Lines:       []*usageLine{line},
					Commitments: []*stackedCommitment{stacked("c1", 1, []int{0, 0}, []*usageLine{line}, 100)},
				}
			},
		},
		{
			name: "usage length differs from windows",
			build: func() *commitmentStack {
				line := quantityLine(40)
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_QUANTITY,
					Windows:     engineWindows(2),
					Lines:       []*usageLine{line},
					Commitments: []*stackedCommitment{stacked("c1", 1, []int{0, 0}, []*usageLine{line}, 100)},
				}
			},
		},
		{
			name: "pool index out of range",
			build: func() *commitmentStack {
				line := quantityLine(40)
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_QUANTITY,
					Windows:     engineWindows(1),
					Lines:       []*usageLine{line},
					Commitments: []*stackedCommitment{stacked("c1", 1, []int{3}, []*usageLine{line}, 100)},
				}
			},
		},
		{
			name: "commitment line missing from the stack",
			build: func() *commitmentStack {
				line := quantityLine(40)
				return &commitmentStack{
					Type:        types.COMMITMENT_TYPE_QUANTITY,
					Windows:     engineWindows(1),
					Lines:       []*usageLine{line},
					Commitments: []*stackedCommitment{stacked("c1", 1, []int{0}, []*usageLine{quantityLine(40)}, 100)},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := allocate(tt.build())
			require.Error(t, err)
			require.Nil(t, got)
			require.True(t, ierr.IsInternal(err), "want internal error, got %v", err)
		})
	}
}

func TestSortCommitmentStack(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2027, 1, d, 0, 0, 0, 0, time.UTC) }
	mk := func(id string, priority int, start time.Time) *stackedCommitment {
		return &stackedCommitment{C: &commitment.Commitment{ID: id, Priority: priority, StartDate: start}}
	}

	tests := []struct {
		name  string
		input []*stackedCommitment
		want  []string
	}{
		{
			name:  "priority ascending",
			input: []*stackedCommitment{mk("c1", 3, day(1)), mk("c2", 1, day(5)), mk("c3", 2, day(2))},
			want:  []string{"c2", "c3", "c1"},
		},
		{
			name:  "earlier start breaks priority ties",
			input: []*stackedCommitment{mk("c1", 1, day(9)), mk("c2", 1, day(3))},
			want:  []string{"c2", "c1"},
		},
		{
			name:  "lower id breaks full ties",
			input: []*stackedCommitment{mk("c9", 1, day(3)), mk("c2", 1, day(3)), mk("c5", 1, day(3))},
			want:  []string{"c2", "c5", "c9"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortCommitmentStack(tt.input)
			got := make([]string, len(tt.input))
			for i, sc := range tt.input {
				got[i] = sc.C.ID
			}

			require.Equal(t, tt.want, got)
		})
	}
}
