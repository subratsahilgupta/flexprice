package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// DashboardAnalyticsRequest represents a request for usage analytics from the customer dashboard
// The ExternalCustomerID is implicitly derived from the authentication context
type DashboardAnalyticsRequest struct {
	FeatureIDs      []string            `json:"feature_ids,omitempty" example:"feat_123,feat_456"`
	Sources         []string            `json:"sources,omitempty" example:"api,web"`
	StartTime       time.Time           `json:"start_time,omitempty" example:"2024-01-01T00:00:00Z"`
	EndTime         time.Time           `json:"end_time,omitempty" example:"2024-01-31T23:59:59Z"`
	GroupBy         []string            `json:"group_by,omitempty" example:"source,feature_id"`
	WindowSize      types.WindowSize    `json:"window_size,omitempty" example:"DAY"`
	Expand          []string            `json:"expand,omitempty" example:"price,meter,feature"`
	PropertyFilters map[string][]string `json:"property_filters,omitempty"`
}

// ToInternalRequest converts the dashboard analytics request to an internal GetUsageAnalyticsRequest
// with the customer ID injected from the authentication context
func (r *DashboardAnalyticsRequest) ToInternalRequest(externalCustomerID string) *GetUsageAnalyticsRequest {
	return &GetUsageAnalyticsRequest{
		ExternalCustomerID: externalCustomerID,
		FeatureIDs:         r.FeatureIDs,
		Sources:            r.Sources,
		StartTime:          r.StartTime,
		EndTime:            r.EndTime,
		GroupBy:            r.GroupBy,
		WindowSize:         r.WindowSize,
		Expand:             r.Expand,
		PropertyFilters:    r.PropertyFilters,
	}
}

// DashboardCostAnalyticsRequest represents a request for cost analytics from the customer dashboard
// The ExternalCustomerID is implicitly derived from the authentication context
type DashboardCostAnalyticsRequest struct {
	FeatureIDs []string  `json:"feature_ids,omitempty" example:"feat_123,feat_456"`
	StartTime  time.Time `json:"start_time" binding:"required" example:"2024-01-01T00:00:00Z"`
	EndTime    time.Time `json:"end_time" binding:"required" example:"2024-01-31T23:59:59Z"`
}

// ToInternalRequest converts the dashboard cost analytics request to an internal GetCostAnalyticsRequest
// with the customer ID injected from the authentication context
func (r *DashboardCostAnalyticsRequest) ToInternalRequest(externalCustomerID string) *GetCostAnalyticsRequest {
	return &GetCostAnalyticsRequest{
		ExternalCustomerID: externalCustomerID,
		FeatureIDs:         r.FeatureIDs,
		StartTime:          r.StartTime,
		EndTime:            r.EndTime,
	}
}

// DashboardPaginatedRequest represents a paginated request from the customer dashboard
type DashboardPaginatedRequest struct {
	Page  int `form:"page" json:"page" example:"1"`
	Limit int `form:"limit" json:"limit" example:"20"`
}
