package service

import (
	"testing"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/require"
)

// TestAllTemporalScheduleConfigsMatchServerScheduleIDs keeps schedule config ids aligned
// with types.AllTemporalServerScheduleIDs (used for schedule_id validation). A config
// entry always exists for every managed schedule even when it is flag-gated off — gating
// happens in EnsureSchedules/isScheduleEnabled, not by omitting the config entry.
func TestAllTemporalScheduleConfigsMatchServerScheduleIDs(t *testing.T) {
	t.Parallel()
	configs := AllTemporalScheduleConfigs(nil)
	ids := types.AllTemporalServerScheduleIDs()
	require.Equal(t, len(ids), len(configs), "each managed schedule must have a config entry")

	expected := make(map[types.ScheduleID]struct{}, len(ids))
	for _, id := range ids {
		expected[id] = struct{}{}
	}
	for _, cfg := range configs {
		_, ok := expected[cfg.ID]
		require.True(t, ok, "config id %q not in AllTemporalServerScheduleIDs", cfg.ID)
		delete(expected, cfg.ID)
	}
	require.Empty(t, expected, "missing schedule config ids")
}

// TestIsScheduleEnabled_RevenueRollupOffByDefault asserts the revenue-rollup schedule
// stays disabled unless analytics.revenue_rollup.enabled is explicitly set, and that
// every other managed schedule remains enabled regardless of cfg.
func TestIsScheduleEnabled_RevenueRollupOffByDefault(t *testing.T) {
	t.Parallel()

	require.False(t, isScheduleEnabled(types.ScheduleIDRevenueRollup, nil), "nil cfg must not enable revenue rollup")
	require.False(t, isScheduleEnabled(types.ScheduleIDRevenueRollup, &config.Configuration{}), "zero-value cfg must not enable revenue rollup")

	enabledCfg := &config.Configuration{}
	enabledCfg.Analytics.RevenueRollup.Enabled = true
	require.True(t, isScheduleEnabled(types.ScheduleIDRevenueRollup, enabledCfg), "explicit enable must gate the schedule on")

	for _, id := range types.AllTemporalServerScheduleIDs() {
		if id == types.ScheduleIDRevenueRollup {
			continue
		}
		require.True(t, isScheduleEnabled(id, nil), "schedule %q must stay enabled by default", id)
	}
}
