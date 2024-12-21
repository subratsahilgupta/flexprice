package builder

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
)

// define a context with a tenant ID to be used in all tests
var ctx = context.WithValue(context.Background(), types.CtxTenantID, types.DefaultTenantID)

func TestQueryBuilder_WithBaseFilters(t *testing.T) {
	tests := []struct {
		name     string
		params   *events.UsageParams
		wantSQL  string
		wantArgs map[string]interface{}
	}{
		{
			name: "base filters with all params",
			params: &events.UsageParams{
				EventName:          "audio_transcription",
				StartTime:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				EndTime:            time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				CustomerID:         "cust_123",
				ExternalCustomerID: "ext_123",
			},
			wantSQL: "WITH base_events AS (SELECT id, timestamp, properties FROM events WHERE event_name = :event_name AND tenant_id = :tenant_id AND timestamp >= :start_time AND timestamp < :end_time AND external_customer_id = :external_customer_id AND customer_id = :customer_id)",
			wantArgs: map[string]interface{}{
				"event_name":           "audio_transcription",
				"start_time":           "2024-01-01T00:00:00Z",
				"end_time":             "2024-01-02T00:00:00Z",
				"external_customer_id": "ext_123",
				"customer_id":          "cust_123",
				"tenant_id":            types.DefaultTenantID,
			},
		},
		{
			name: "base filters without customer ID",
			params: &events.UsageParams{
				EventName: "api_calls",
				StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			wantSQL: "WITH base_events AS (SELECT id, timestamp, properties FROM events WHERE event_name = :event_name AND tenant_id = :tenant_id AND timestamp >= :start_time AND timestamp < :end_time)",
			wantArgs: map[string]interface{}{
				"event_name": "api_calls",
				"start_time": "2024-01-01T00:00:00Z",
				"end_time":   "2024-01-02T00:00:00Z",
				"tenant_id":  types.DefaultTenantID,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.WithBaseFilters(ctx, tt.params)
			sql, args := qb.Build()
			assert.Contains(t, sql, tt.wantSQL)
			if tt.wantArgs != nil {
				assert.Equal(t, tt.wantArgs, args)
			}
		})
	}
}

func TestQueryBuilder_WithFilterGroups(t *testing.T) {
	tests := []struct {
		name        string
		meterConfig *meter.Meter
		groups      []events.FilterGroup
		wantCTEs    []string
		wantFilters []string
	}{
		{
			name: "multiple filter groups with different priorities",
			meterConfig: &meter.Meter{
				EventName: "audio_transcription",
				Filters: []meter.Filter{
					{Key: "test_group", Values: []string{"group_0", "group_1"}},
					{Key: "audio_model", Values: []string{"whisper", "deepgram"}},
				},
			},
			groups: []events.FilterGroup{
				{
					ID:       "1",
					Priority: 2,
					Filters: map[string][]string{
						"test_group":  {"group_0"},
						"audio_model": {"whisper"},
					},
				},
				{
					ID:       "2",
					Priority: 1,
					Filters: map[string][]string{
						"audio_model": {"whisper"},
					},
				},
			},
			wantCTEs: []string{
				"base_events",
				"filter_matches",
				"matched_events",
			},
			wantFilters: []string{
				"JSONExtractString(properties, 'test_group') IN ('group_0')",
				"JSONExtractString(properties, 'audio_model') IN ('whisper')",
			},
		},
		{
			name: "single filter group",
			meterConfig: &meter.Meter{
				EventName: "api_calls",
				Filters: []meter.Filter{
					{Key: "endpoint", Values: []string{"/api/v1", "/api/v2"}},
				},
			},
			groups: []events.FilterGroup{
				{
					ID:       "1",
					Priority: 1,
					Filters: map[string][]string{
						"endpoint": {"/api/v1"},
					},
				},
			},
			wantCTEs: []string{
				"base_events",
				"filter_matches",
				"matched_events",
			},
			wantFilters: []string{
				"JSONExtractString(properties, 'endpoint') IN ('/api/v1')",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.WithBaseFilters(ctx, &events.UsageParams{EventName: tt.meterConfig.EventName})
			qb.WithFilterGroups(ctx, tt.groups)
			sql, _ := qb.Build()

			// Check CTEs are present and in correct order
			for i, cte := range tt.wantCTEs {
				assert.Contains(t, sql, cte)
				if i > 0 {
					prevCTEPos := strings.Index(sql, tt.wantCTEs[i-1])
					currentCTEPos := strings.Index(sql, cte)
					assert.True(t, prevCTEPos < currentCTEPos)
				}
			}

			// Check filters are present
			for _, filter := range tt.wantFilters {
				assert.Contains(t, sql, filter)
			}
		})
	}
}

func TestQueryBuilder_WithAggregation(t *testing.T) {
	tests := []struct {
		name         string
		aggType      types.AggregationType
		propertyName string
		wantSQL      string
	}{
		{
			name:    "count aggregation",
			aggType: types.AggregationCount,
			wantSQL: "SELECT best_match_group as filter_group_id, COUNT(*) as value FROM matched_events GROUP BY best_match_group ORDER BY best_match_group",
		},
		{
			name:         "sum aggregation",
			aggType:      types.AggregationSum,
			propertyName: "duration",
			wantSQL:      "SELECT best_match_group as filter_group_id, SUM(CAST(JSONExtractString(properties, 'duration') AS Float64)) as value FROM matched_events GROUP BY best_match_group ORDER BY best_match_group",
		},
		{
			name:         "avg aggregation",
			aggType:      types.AggregationAvg,
			propertyName: "response_time",
			wantSQL:      "SELECT best_match_group as filter_group_id, AVG(CAST(JSONExtractString(properties, 'response_time') AS Float64)) as value FROM matched_events GROUP BY best_match_group ORDER BY best_match_group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.WithBaseFilters(ctx, &events.UsageParams{EventName: "test"})
			qb.WithFilterGroups(ctx, []events.FilterGroup{{ID: "1"}})
			qb.WithAggregation(ctx, tt.aggType, tt.propertyName)
			sql, _ := qb.Build()
			assert.Contains(t, sql, tt.wantSQL)
		})
	}
}

func TestQueryBuilder_CompleteFlow(t *testing.T) {
	tests := []struct {
		name        string
		params      *events.UsageParams
		meterConfig *meter.Meter
		groups      []events.FilterGroup
		wantCTEs    []string
		wantArgs    map[string]interface{}
	}{
		{
			name: "complete flow with multiple filter groups and sum aggregation",
			params: &events.UsageParams{
				EventName:          "audio_transcription",
				StartTime:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				EndTime:            time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				CustomerID:         "cust_123",
				ExternalCustomerID: "ext_123",
			},
			meterConfig: &meter.Meter{
				EventName: "audio_transcription",
				Aggregation: meter.Aggregation{
					Type:  types.AggregationSum,
					Field: "duration",
				},
				Filters: []meter.Filter{
					{Key: "test_group", Values: []string{"group_0", "group_1"}},
					{Key: "audio_model", Values: []string{"whisper", "deepgram"}},
				},
			},
			groups: []events.FilterGroup{
				{
					ID:       "1",
					Priority: 2,
					Filters: map[string][]string{
						"test_group":  {"group_0"},
						"audio_model": {"whisper"},
					},
				},
				{
					ID:       "2",
					Priority: 1,
					Filters: map[string][]string{
						"audio_model": {"whisper"},
					},
				},
			},
			wantCTEs: []string{
				"base_events",
				"filter_matches",
				"matched_events",
			},
			wantArgs: map[string]interface{}{
				"event_name":           "audio_transcription",
				"start_time":           "2024-01-01T00:00:00Z",
				"end_time":             "2024-01-02T00:00:00Z",
				"external_customer_id": "ext_123",
				"customer_id":          "cust_123",
				"tenant_id":            types.DefaultTenantID,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.WithBaseFilters(ctx, tt.params)
			qb.WithFilterGroups(ctx, tt.groups)
			qb.WithAggregation(ctx, tt.meterConfig.Aggregation.Type, tt.meterConfig.Aggregation.Field)
			sql, args := qb.Build()

			// Check for presence of each CTE part without WITH prefix
			for i, cte := range tt.wantCTEs {
				ctePart := strings.TrimPrefix(cte, "WITH ")
				assert.Contains(t, sql, ctePart)
				if i > 0 {
					prevCTE := strings.TrimPrefix(tt.wantCTEs[i-1], "WITH ")
					assert.True(t, strings.Index(sql, prevCTE) < strings.Index(sql, ctePart))
				}
			}

			assert.Equal(t, tt.wantArgs, args)
		})
	}
}
