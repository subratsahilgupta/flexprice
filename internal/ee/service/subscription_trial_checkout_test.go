package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrialConversionPeriod(t *testing.T) {
	start := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		cycle      types.BillingCycle
		wantAnchor time.Time
		wantEnd    time.Time
	}{
		{"calendar", types.BillingCycleCalendar, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"anniversary", types.BillingCycleAnniversary, start, time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &subscription.Subscription{BillingCycle: tt.cycle, BillingPeriod: types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1, Timezone: "UTC"}
			anchor, end, err := trialConversionPeriod(sub, start)
			require.NoError(t, err)
			assert.True(t, anchor.Equal(tt.wantAnchor), "anchor %s", anchor)
			assert.True(t, end.Equal(tt.wantEnd), "end %s", end)
			assert.True(t, sub.BillingAnchor.IsZero(), "helper must not mutate the subscription")
		})
	}
}
