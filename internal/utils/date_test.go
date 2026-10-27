package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLocalDay(t *testing.T) {
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	require.NoError(t, err)
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)
	instant := time.Date(2026, time.January, 31, 18, 30, 0, 0, time.UTC)

	tests := []struct {
		name string
		loc  *time.Location
		want time.Time
	}{
		{name: "ahead of UTC crosses into the next day", loc: kolkata, want: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)},
		{name: "UTC keeps the day", loc: time.UTC, want: time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)},
		{name: "behind UTC keeps the earlier day", loc: losAngeles, want: time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)},
		{name: "nil location is UTC", loc: nil, want: time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, LocalDay(instant, tt.loc))
		})
	}
}
