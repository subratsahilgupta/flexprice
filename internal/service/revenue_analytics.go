package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/shopspring/decimal"
)

type RevenueAnalyticsService = interfaces.RevenueAnalyticsService

type revenueAnalyticsService struct {
	ServiceParams
}

func NewRevenueAnalyticsService(params ServiceParams) RevenueAnalyticsService {
	return &revenueAnalyticsService{
		ServiceParams: params,
	}
}

// GetDetailedCostAnalytics retrieves detailed cost analytics with derived metrics
func (s *revenueAnalyticsService) GetDetailedCostAnalytics(
	ctx context.Context,
	req *dto.GetCostAnalyticsRequest,
) (*dto.GetDetailedCostAnalyticsResponse, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid cost analytics request").
			Mark(ierr.ErrValidation)
	}

	// Define services
	featureUsageTrackingService := NewFeatureUsageTrackingService(s.ServiceParams, s.EventRepo, s.FeatureUsageRepo)
	costsheetUsageTrackingService := NewCostSheetUsageTrackingService(s.ServiceParams, s.EventRepo, s.CostSheetUsageRepo)

	// 1. Fetch cost analytics using the costsheet usage tracking service
	var costAnalytics *dto.GetCostAnalyticsResponse
	costAnalytics, err := costsheetUsageTrackingService.GetCostSheetUsageAnalytics(ctx, req)
	if err != nil {
		s.Logger.Warnw("failed to fetch cost analytics", "error", err)
		costAnalytics = nil
	}

	// 2. Fetch revenue analytics from feature usage tracking
	var revenueAnalytics *dto.GetUsageAnalyticsResponse
	revenueReq := &dto.GetUsageAnalyticsRequest{
		ExternalCustomerID: req.ExternalCustomerID,
		FeatureIDs:         req.FeatureIDs,
		StartTime:          req.StartTime,
		EndTime:            req.EndTime,
	}
	revenueAnalytics, err = featureUsageTrackingService.GetDetailedUsageAnalyticsV2(ctx, revenueReq)
	if err != nil {
		s.Logger.Warnw("failed to fetch revenue analytics", "error", err)
		revenueAnalytics = nil
	}

	// 3. Build response with derived metrics
	response := &dto.GetDetailedCostAnalyticsResponse{
		CostAnalytics: []dto.CostAnalyticItem{},
		TotalCost:     decimal.Zero,
		TotalRevenue:  decimal.Zero,
		Margin:        decimal.Zero,
		MarginPercent: decimal.Zero,
		ROI:           decimal.Zero,
		ROIPercent:    decimal.Zero,
		Currency:      "USD", // Default currency
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
	}

	// Populate cost analytics if available
	if costAnalytics != nil {
		response.CostAnalytics = costAnalytics.CostAnalytics
		response.TotalCost = costAnalytics.TotalCost
		if costAnalytics.Currency != "" {
			response.Currency = costAnalytics.Currency
		}
		response.StartTime = costAnalytics.StartTime
		response.EndTime = costAnalytics.EndTime
	}

	// Calculate total revenue from revenue analytics
	if revenueAnalytics != nil {
		for _, item := range revenueAnalytics.Items {
			response.TotalRevenue = response.TotalRevenue.Add(item.TotalCost) // TotalCost in usage analytics represents revenue
		}
		if revenueAnalytics.Currency != "" && costAnalytics == nil {
			// Use revenue currency if cost analytics is not available
			response.Currency = revenueAnalytics.Currency
		}
	}

	// Calculate derived metrics if both cost and revenue are available
	if costAnalytics != nil && revenueAnalytics != nil {
		response.Margin = response.TotalRevenue.Sub(response.TotalCost)

		if !response.TotalRevenue.IsZero() {
			response.MarginPercent = response.Margin.Div(response.TotalRevenue).Mul(decimal.NewFromInt(100))
		}

		if !response.TotalCost.IsZero() {
			response.ROI = response.Margin.Div(response.TotalCost)
			response.ROIPercent = response.ROI.Mul(decimal.NewFromInt(100))
		}
	}

	// Return response even if cost or revenue analytics failed
	return response, nil
}
