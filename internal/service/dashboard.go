package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/types"
)

// DashboardService provides dashboard functionality
type DashboardService interface {
	GetRevenues(ctx context.Context, req dto.DashboardRevenuesRequest) (*dto.DashboardRevenuesResponse, error)
}

type dashboardService struct {
	ServiceParams
}

// NewDashboardService creates a new dashboard service
func NewDashboardService(
	params ServiceParams,
) DashboardService {
	return &dashboardService{
		ServiceParams: params,
	}
}

// GetRevenues returns dashboard revenues data
func (s *dashboardService) GetRevenues(ctx context.Context, req dto.DashboardRevenuesRequest) (*dto.DashboardRevenuesResponse, error) {
	response := &dto.DashboardRevenuesResponse{}

	// Revenue Trend
	if req.RevenueTrend != nil {
		revenueTrend, err := s.getRevenueTrend(ctx, req.RevenueTrend)
		if err != nil {
			s.Logger.Errorw("failed to get revenue trend", "error", err)
			// Continue with other sections even if this fails
		} else {
			response.RevenueTrend = revenueTrend
		}
	}

	// Recent Subscriptions - always fetch
	recentSubs, err := s.getRecentSubscriptions(ctx)
	if err != nil {
		s.Logger.Errorw("failed to get recent subscriptions", "error", err)
		// Continue with other sections even if this fails
	} else {
		response.RecentSubscriptions = recentSubs
	}

	// Invoice Payment Status - always fetch
	paymentStatus, err := s.getInvoicePaymentStatus(ctx)
	if err != nil {
		s.Logger.Errorw("failed to get invoice payment status", "error", err)
		// Continue with other sections even if this fails
	} else {
		response.InvoicePaymentStatus = paymentStatus
	}

	return response, nil
}

// getRevenueTrend calculates revenue trend data using repository
func (s *dashboardService) getRevenueTrend(ctx context.Context, req *dto.RevenueTrendRequest) (*dto.RevenueTrendResponse, error) {
	// Call repository method
	windows, err := s.InvoiceRepo.GetRevenueTrend(ctx, types.WindowSize(req.WindowSize), *req.WindowCount)
	if err != nil {
		return nil, err
	}

	// Convert to DTO
	revenueWindows := make([]types.RevenueWindow, 0, len(windows))
	for _, w := range windows {
		label := s.formatWindowLabel(w.WindowStart, w.WindowEnd, types.WindowSize(req.WindowSize))
		revenueWindows = append(revenueWindows, types.RevenueWindow{
			WindowStart:  w.WindowStart,
			WindowEnd:    w.WindowEnd,
			WindowLabel:  label,
			TotalRevenue: w.Revenue,
		})
	}

	return &dto.RevenueTrendResponse{
		Windows:     revenueWindows,
		WindowSize:  req.WindowSize,
		WindowCount: *req.WindowCount,
	}, nil
}

// formatWindowLabel formats a window label based on window size
func (s *dashboardService) formatWindowLabel(start, end time.Time, windowSize types.WindowSize) string {
	switch windowSize {
	case types.WindowSizeMonth:
		return start.Format("Jan 2006")
	case types.WindowSizeWeek:
		return fmt.Sprintf("Week of %s", start.Format("Jan 2"))
	case types.WindowSizeDay:
		return start.Format("Jan 2, 2006")
	case types.WindowSizeHour:
		return start.Format("Jan 2, 3:04 PM")
	default:
		return start.Format("Jan 2, 2006")
	}
}

// getRecentSubscriptions gets recent subscriptions grouped by plan using repository
func (s *dashboardService) getRecentSubscriptions(ctx context.Context) (*dto.RecentSubscriptionsResponse, error) {
	// Call repository method
	planCounts, err := s.SubRepo.GetRecentSubscriptionsByPlan(ctx)
	if err != nil {
		return nil, err
	}

	// Convert to DTO
	byPlan := make([]types.SubscriptionPlanCount, 0, len(planCounts))
	totalCount := 0
	for _, pc := range planCounts {
		byPlan = append(byPlan, types.SubscriptionPlanCount{
			PlanID:   pc.PlanID,
			PlanName: pc.PlanName,
			Count:    pc.Count,
		})
		totalCount += pc.Count
	}

	now := time.Now().UTC()
	periodStart := now.AddDate(0, 0, -7) // 7 days ago
	return &dto.RecentSubscriptionsResponse{
		TotalCount:  totalCount,
		ByPlan:      byPlan,
		PeriodStart: periodStart,
		PeriodEnd:   now,
	}, nil
}

// getInvoicePaymentStatus gets invoice payment status counts using repository
func (s *dashboardService) getInvoicePaymentStatus(ctx context.Context) (*dto.InvoicePaymentStatusResponse, error) {
	// Call repository method
	status, err := s.InvoiceRepo.GetInvoicePaymentStatus(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	return &dto.InvoicePaymentStatusResponse{
		Paid:        status.Succeeded,
		Pending:     status.Pending,
		Failed:      status.Failed,
		PeriodStart: now,
		PeriodEnd:   now,
	}, nil
}
