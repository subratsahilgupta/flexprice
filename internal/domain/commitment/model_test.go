package commitment

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestCommitment_IsActiveAt(t *testing.T) {
	start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	termAt := start.AddDate(0, 6, 0)
	base := &Commitment{StartDate: start, EndDate: lo.ToPtr(end)}
	terminated := &Commitment{StartDate: start, EndDate: lo.ToPtr(end), TerminatedAt: &termAt}
	openEnded := &Commitment{StartDate: start}
	openTerminated := &Commitment{StartDate: start, TerminatedAt: &termAt}

	tests := []struct {
		name string
		c    *Commitment
		at   time.Time
		want bool
	}{
		{"open-ended before start", openEnded, start.Add(-time.Nanosecond), false},
		{"open-ended decades on", openEnded, start.AddDate(50, 0, 0), true},
		{"open-ended at terminated_at", openTerminated, termAt, false},
		{"before start", base, start.Add(-time.Nanosecond), false},
		{"at start", base, start, true},
		{"one instant before end", base, end.Add(-time.Nanosecond), true},
		{"at end", base, end, false},
		{"before terminated_at", terminated, termAt.Add(-time.Nanosecond), true},
		{"at terminated_at", terminated, termAt, false},
		{"after terminated_at", terminated, termAt.Add(time.Hour), false},
		{"nil", nil, start, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.c.IsActiveAt(tt.at))
		})
	}
}

func TestCommitment_EffectiveEnd(t *testing.T) {
	start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	early := start.AddDate(0, 6, 0)
	late := end.AddDate(0, 1, 0)

	require.Equal(t, end, (&Commitment{StartDate: start, EndDate: lo.ToPtr(end)}).EffectiveEnd())
	require.Equal(t, early, (&Commitment{StartDate: start, EndDate: lo.ToPtr(end), TerminatedAt: &early}).EffectiveEnd())
	require.Equal(t, end, (&Commitment{StartDate: start, EndDate: lo.ToPtr(end), TerminatedAt: &late}).EffectiveEnd())
	require.Equal(t, types.EndOfTime, (&Commitment{StartDate: start}).EffectiveEnd())
	require.Equal(t, early, (&Commitment{StartDate: start, TerminatedAt: &early}).EffectiveEnd())
}

func TestCommitment_Unending(t *testing.T) {
	start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)

	require.True(t, (&Commitment{StartDate: start}).Unending())
	require.False(t, (&Commitment{StartDate: start, TerminatedAt: &end}).Unending())
	require.False(t, (&Commitment{StartDate: start, EndDate: &end}).Unending())
}

func TestCommitment_EndedBy(t *testing.T) {
	start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	termAt := start.AddDate(0, 6, 0)
	beforeStart := start.AddDate(0, -1, 0)
	base := &Commitment{StartDate: start, EndDate: lo.ToPtr(end)}
	terminated := &Commitment{StartDate: start, EndDate: lo.ToPtr(end), TerminatedAt: &termAt}
	neverStarted := &Commitment{StartDate: start, EndDate: lo.ToPtr(end), TerminatedAt: &beforeStart}

	tests := []struct {
		name string
		c    *Commitment
		at   time.Time
		want bool
	}{
		{"before start", base, beforeStart, false},
		{"mid term", base, termAt, false},
		{"one instant before end", base, end.Add(-time.Nanosecond), false},
		{"at end", base, end, true},
		{"before terminated_at", terminated, termAt.Add(-time.Nanosecond), false},
		{"at terminated_at", terminated, termAt, true},
		{"terminated before start", neverStarted, beforeStart.Add(-time.Hour), true},
		{"open-ended decades on", &Commitment{StartDate: start}, start.AddDate(50, 0, 0), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.c.EndedBy(tt.at))
		})
	}
}
