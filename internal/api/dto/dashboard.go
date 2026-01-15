package dto

import (
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// DashboardRevenuesRequest represents the request for dashboard revenues API
type DashboardRevenuesRequest struct {
	RevenueTrend *RevenueTrendRequest `json:"revenue_trend,omitempty"`
}

// Validate validates the dashboard revenues request
func (r *DashboardRevenuesRequest) Validate() error {
	if r.RevenueTrend != nil {
		return r.RevenueTrend.Validate()
	}
	return nil
}

// RevenueTrendRequest represents parameters for revenue trend section
type RevenueTrendRequest struct {
	WindowSize  types.WindowSize `json:"window_size,omitempty"`  //by default, it's MONTH
	WindowCount *int             `json:"window_count,omitempty"` //by default, it's 3
}

// Validate validates the revenue trend request
func (r *RevenueTrendRequest) Validate() error {
	// Validate window_size if provided
	if r.WindowSize != "" {
		if err := r.WindowSize.Validate(); err != nil {
			return err
		}
	}

	// Validate window_count if provided
	if r.WindowCount != nil && *r.WindowCount <= 0 {
		return ierr.NewError("window_count must be >= 1").
			WithHint("window_count must be a positive integer").
			WithReportableDetails(map[string]interface{}{
				"window_count": *r.WindowCount,
			}).
			Mark(ierr.ErrValidation)
	}

	return nil
}

// DashboardRevenuesResponse represents the response for dashboard revenues API
type DashboardRevenuesResponse struct {
	RevenueTrend         *RevenueTrendResponse         `json:"revenue_trend,omitempty"`
	RecentSubscriptions  *RecentSubscriptionsResponse  `json:"recent_subscriptions,omitempty"`
	InvoicePaymentStatus *InvoicePaymentStatusResponse `json:"invoice_payment_status,omitempty"`
}

// RevenueTrendResponse represents revenue trend data
type RevenueTrendResponse struct {
	Windows     []types.RevenueWindow `json:"windows"`
	WindowSize  types.WindowSize      `json:"window_size"`
	WindowCount int                   `json:"window_count"`
}

// RecentSubscriptionsResponse represents recent subscriptions data
type RecentSubscriptionsResponse struct {
	TotalCount  int                           `json:"total_count"`
	ByPlan      []types.SubscriptionPlanCount `json:"by_plan"`
	PeriodStart time.Time                     `json:"period_start"`
	PeriodEnd   time.Time                     `json:"period_end"`
}

// InvoicePaymentStatusResponse represents invoice payment status counts
type InvoicePaymentStatusResponse struct {
	Paid        int       `json:"paid"`
	Pending     int       `json:"pending"`
	Failed      int       `json:"failed"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
}
