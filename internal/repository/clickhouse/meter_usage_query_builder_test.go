package clickhouse

import (
	"strings"
	"testing"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/require"
)

// TestBuildDetailedGroupByColumns_ExternalCustomerID is a focused test for
// the Change-2 additive extension: "external_customer_id" is a real, indexed
// column on meter_usage, so it must be selected/grouped directly (no
// JSONExtract), unlike "properties.<field>".
func TestBuildDetailedGroupByColumns_ExternalCustomerID(t *testing.T) {
	qb := NewMeterUsageQueryBuilder()

	result, err := qb.BuildDetailedGroupByColumns(&events.MeterUsageDetailedAnalyticsParams{
		GroupBy: []string{"external_customer_id"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"external_customer_id"}, result.Columns)
	require.Equal(t, []string{"external_customer_id"}, result.Aliases)
	require.Equal(t, "external_customer_id", result.FieldMapping["external_customer_id"])

	for _, col := range result.Columns {
		require.NotContains(t, col, "JSONExtractString", "external_customer_id is a real column, not a JSON property")
	}
}

// TestBuildDetailedGroupByColumns_ExternalCustomerIDAlongsideExisting proves
// the new entry composes with the pre-existing allowlist (meter_id, source,
// properties.*) without disturbing their output — purely additive.
func TestBuildDetailedGroupByColumns_ExternalCustomerIDAlongsideExisting(t *testing.T) {
	qb := NewMeterUsageQueryBuilder()

	result, err := qb.BuildDetailedGroupByColumns(&events.MeterUsageDetailedAnalyticsParams{
		GroupBy: []string{"meter_id", "source", "external_customer_id", "properties.region"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"meter_id", "source", "external_customer_id", "JSONExtractString(properties, 'region')"}, result.Columns)
	require.Equal(t, "meter_id", result.FieldMapping["meter_id"])
	require.Equal(t, "source", result.FieldMapping["source"])
	require.Equal(t, "external_customer_id", result.FieldMapping["external_customer_id"])
	require.Equal(t, "prop_region", result.FieldMapping["properties.region"])
}

// TestBuildDetailedPointsQuery_NarrowsByExternalCustomerID proves the
// per-group points sub-query added by Change 2 narrows to the same customer
// as the aggregate row it belongs to, mirroring the existing Source narrowing.
func TestBuildDetailedPointsQuery_NarrowsByExternalCustomerID(t *testing.T) {
	qb := NewMeterUsageQueryBuilder()

	params := &events.MeterUsageDetailedAnalyticsParams{
		TenantID:      "tenant_1",
		EnvironmentID: "env_1",
		WindowSize:    types.WindowSizeDay,
	}
	groupByResult, err := qb.BuildDetailedGroupByColumns(&events.MeterUsageDetailedAnalyticsParams{
		GroupBy: []string{"external_customer_id"},
	})
	require.NoError(t, err)

	result := &events.MeterUsageDetailedResult{ExternalCustomerID: "cust_1"}
	query, args := qb.BuildDetailedPointsQuery(params, result, groupByResult)
	require.Contains(t, query, "external_customer_id = ?")
	require.Contains(t, args, "cust_1")
	require.Equal(t, 1, strings.Count(query, "external_customer_id = ?"))
}

func TestBuildDetailedGroupByColumns_RejectsUnknownEntry(t *testing.T) {
	qb := NewMeterUsageQueryBuilder()

	_, err := qb.BuildDetailedGroupByColumns(&events.MeterUsageDetailedAnalyticsParams{
		GroupBy: []string{"not_a_real_column"},
	})
	require.Error(t, err)
}
