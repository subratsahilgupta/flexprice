package clickhouse

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// day parses a "2006-01-02" date string into a UTC time.Time for test fixtures.
func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// TestBuildDailyUsageQuery_GroupsByDayWithRLS proves the builder groups by
// meter and day, scopes by tenant/environment/meter,
// and honors UseFinal — mirroring BuildDetailedWhereClause / BuildFinalClause
// used elsewhere in this file, without touching any existing method.
func TestBuildDailyUsageQuery_GroupsByDayWithRLS(t *testing.T) {
	qb := NewMeterUsageQueryBuilder()

	q, args := qb.BuildDailyUsageQuery(&events.DailyUsageParams{
		TenantID:      "t1",
		EnvironmentID: "e1",
		MeterIDs:      []string{"m1"},
		StartTime:     day("2026-09-01"),
		EndTime:       day("2026-10-01"),
		UseFinal:      true,
	})

	assert.Contains(t, q, "toStartOfDay(timestamp")
	assert.Contains(t, q, "tenant_id = ?")
	assert.Contains(t, q, "environment_id = ?")
	assert.Contains(t, q, "GROUP BY meter_id, day")
	assert.Contains(t, q, "ORDER BY meter_id ASC, day ASC")
	assert.Contains(t, q, "FINAL")

	assert.Contains(t, args, "t1")
	assert.Contains(t, args, "e1")
	assert.Contains(t, args, "m1")
}

// TestBuildDailyUsageQuery_NoFinalWhenNotRequested proves UseFinal=false
// omits the FINAL clause, matching BuildFinalClause's existing behavior.
func TestBuildDailyUsageQuery_NoFinalWhenNotRequested(t *testing.T) {
	qb := NewMeterUsageQueryBuilder()

	q, args := qb.BuildDailyUsageQuery(&events.DailyUsageParams{
		TenantID:      "t1",
		EnvironmentID: "e1",
		MeterIDs:      []string{"m1"},
		StartTime:     day("2026-09-01"),
		EndTime:       day("2026-10-01"),
		UseFinal:      false,
	})

	assert.NotContains(t, q, "FINAL")
	require.NotEmpty(t, args)
}

// TestBuildDailyUsageQuery_SumsQtyTotal proves the aggregation
// expression is SUM(qty_total) — the billable pre-materialized quantity —
// not a JSONExtract over properties.
func TestBuildDailyUsageQuery_SumsQtyTotal(t *testing.T) {
	qb := NewMeterUsageQueryBuilder()

	q, _ := qb.BuildDailyUsageQuery(&events.DailyUsageParams{
		TenantID:      "t1",
		EnvironmentID: "e1",
		MeterIDs:      []string{"m1"},
		StartTime:     day("2026-09-01"),
		EndTime:       day("2026-10-01"),
	})

	assert.Contains(t, q, "SUM(qty_total)")
}

// TestBuildDailyUsageQuery_BoundsMaxMemoryUsage proves the query
// carries the same inline 90GB max_memory_usage bound as its sibling
// meter_usage.go queries (AGENTS.md: every ClickHouse query bounded by 90GB).
func TestBuildDailyUsageQuery_BoundsMaxMemoryUsage(t *testing.T) {
	qb := NewMeterUsageQueryBuilder()

	q, _ := qb.BuildDailyUsageQuery(&events.DailyUsageParams{
		TenantID:      "t1",
		EnvironmentID: "e1",
		MeterIDs:      []string{"m1"},
		StartTime:     day("2026-09-01"),
		EndTime:       day("2026-10-01"),
		UseFinal:      true,
	})

	assert.Contains(t, q, "SETTINGS")
	assert.Contains(t, q, "max_memory_usage = 96636764160")
	assert.Contains(t, q, "do_not_merge_across_partitions_select_final = 1")
}

// TestBuildDailyUsageQuery_BatchesMeters proves several meters go out in one
// query, keyed by meter in the projection. One query per line item is what made
// a full rollup pass take hours.
func TestBuildDailyUsageQuery_BatchesMeters(t *testing.T) {
	qb := NewMeterUsageQueryBuilder()

	q, args := qb.BuildDailyUsageQuery(&events.DailyUsageParams{
		TenantID:      "t1",
		EnvironmentID: "e1",
		MeterIDs:      []string{"m1", "m2", "m3"},
		StartTime:     day("2026-09-01"),
		EndTime:       day("2026-10-01"),
	})

	assert.Contains(t, q, "meter_id,")
	assert.Contains(t, q, "GROUP BY meter_id, day")
	for _, m := range []string{"m1", "m2", "m3"} {
		assert.Contains(t, args, m, "every meter must reach the query")
	}
}
