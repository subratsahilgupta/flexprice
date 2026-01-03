package service

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/auth"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// CustomerDashboardService provides customer dashboard functionality
type CustomerDashboardService interface {
	// CreateDashboardSession creates a dashboard session URL for a customer
	CreateDashboardSession(ctx context.Context, req dto.CreateDashboardSessionRequest) (*dto.DashboardSessionResponse, error)
	// GetCustomer returns customer info for the dashboard
	GetCustomer(ctx context.Context) (*dto.CustomerResponse, error)
	// UpdateCustomer updates customer info from the dashboard
	UpdateCustomer(ctx context.Context, req dto.UpdateCustomerRequest) (*dto.CustomerResponse, error)
	// GetSubscriptions returns subscriptions for the dashboard customer
	GetSubscriptions(ctx context.Context, req dto.DashboardPaginatedRequest) (*dto.ListSubscriptionsResponse, error)
	// GetSubscription returns a specific subscription
	GetSubscription(ctx context.Context, subscriptionID string) (*dto.SubscriptionResponse, error)
	// GetInvoices returns invoices for the dashboard customer
	GetInvoices(ctx context.Context, req dto.DashboardPaginatedRequest) (*dto.ListInvoicesResponse, error)
	// GetInvoice returns a specific invoice
	GetInvoice(ctx context.Context, invoiceID string) (*dto.InvoiceResponse, error)
	// GetWallets returns wallet balances for the dashboard customer
	GetWallets(ctx context.Context) ([]*dto.WalletBalanceResponse, error)
	// GetWallet returns a specific wallet
	GetWallet(ctx context.Context, walletID string) (*dto.WalletBalanceResponse, error)
	// GetAnalytics returns usage analytics for the dashboard customer
	GetAnalytics(ctx context.Context, req dto.DashboardAnalyticsRequest) (*dto.GetUsageAnalyticsResponse, error)
	// GetCostAnalytics returns cost analytics for the dashboard customer
	GetCostAnalytics(ctx context.Context, req dto.DashboardCostAnalyticsRequest) (*dto.GetDetailedCostAnalyticsResponse, error)
}

type customerDashboardService struct {
	ServiceParams
	customerService CustomerService
}

// NewCustomerDashboardService creates a new customer dashboard service
func NewCustomerDashboardService(
	params ServiceParams,
	customerService CustomerService,
) CustomerDashboardService {
	return &customerDashboardService{
		ServiceParams:   params,
		customerService: customerService,
	}
}

// CreateDashboardSession creates a dashboard session for a customer
func (s *customerDashboardService) CreateDashboardSession(ctx context.Context, req dto.CreateDashboardSessionRequest) (*dto.DashboardSessionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Look up the customer by external ID
	customer, err := s.customerService.GetCustomerByLookupKey(ctx, req.ExternalCustomerID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Customer not found with the provided external_customer_id").
			Mark(ierr.ErrNotFound)
	}

	// Generate the dashboard token using Flexprice Auth provider
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	// Get auth provider
	authProvider := auth.NewFlexpriceAuth(s.ServiceParams.Config)

	token, expiresAt, err := authProvider.GenerateDashboardToken(customer.ID, req.ExternalCustomerID, tenantID, environmentID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to generate dashboard token").
			Mark(ierr.ErrSystem)
	}

	// Build the dashboard URL
	dashboardURL := fmt.Sprintf("%s?token=%s", req.ReturnURL, token)
	if req.ReturnURL == "" {
		dashboardURL = token // Just return the token if no return URL
	}

	return &dto.DashboardSessionResponse{
		URL:       dashboardURL,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

// GetCustomer returns customer info for the dashboard
func (s *customerDashboardService) GetCustomer(ctx context.Context) (*dto.CustomerResponse, error) {

	customerID := types.GetCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}
	customer, err := s.customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}

	return customer, nil
}

// UpdateCustomer updates customer info from the dashboard
func (s *customerDashboardService) UpdateCustomer(ctx context.Context, req dto.UpdateCustomerRequest) (*dto.CustomerResponse, error) {
	customerID := types.GetCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	customer, err := s.customerService.UpdateCustomer(ctx, customerID, req)
	if err != nil {
		return nil, err
	}

	return customer, nil
}

// GetSubscriptions returns subscriptions for the dashboard customer
func (s *customerDashboardService) GetSubscriptions(ctx context.Context, req dto.DashboardPaginatedRequest) (*dto.ListSubscriptionsResponse, error) {
	customerID := types.GetCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	subscriptionService := NewSubscriptionService(s.ServiceParams)

	// Build query filter with pagination
	queryFilter := types.NewDefaultQueryFilter()
	if req.Limit > 0 {
		queryFilter.Limit = &req.Limit
	}
	if req.Page > 0 {
		limit := queryFilter.GetLimit()
		offset := (req.Page - 1) * limit
		queryFilter.Offset = &offset
	}

	filter := &types.SubscriptionFilter{
		CustomerID:  customerID,
		QueryFilter: queryFilter,
	}

	return subscriptionService.ListSubscriptions(ctx, filter)
}

// GetSubscription returns a specific subscription
func (s *customerDashboardService) GetSubscription(ctx context.Context, subscriptionID string) (*dto.SubscriptionResponse, error) {
	customerID := types.GetCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	subscriptionService := NewSubscriptionService(s.ServiceParams)

	// Get the subscription
	subscription, err := subscriptionService.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}

	// Verify it belongs to this customer
	if subscription.CustomerID != customerID {
		return nil, ierr.NewError("subscription not found").
			WithHint("Subscription does not belong to this customer").
			Mark(ierr.ErrNotFound)
	}

	return subscription, nil
}

// GetInvoices returns invoices for the dashboard customer
func (s *customerDashboardService) GetInvoices(ctx context.Context, req dto.DashboardPaginatedRequest) (*dto.ListInvoicesResponse, error) {
	customerID := types.GetCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	invoiceService := NewInvoiceService(s.ServiceParams)

	// Build query filter with pagination
	queryFilter := types.NewDefaultQueryFilter()
	if req.Limit > 0 {
		queryFilter.Limit = &req.Limit
	}
	if req.Page > 0 {
		limit := queryFilter.GetLimit()
		offset := (req.Page - 1) * limit
		queryFilter.Offset = &offset
	}

	filter := &types.InvoiceFilter{
		CustomerID:  customerID,
		QueryFilter: queryFilter,
	}

	return invoiceService.ListInvoices(ctx, filter)
}

// GetInvoice returns a specific invoice
func (s *customerDashboardService) GetInvoice(ctx context.Context, invoiceID string) (*dto.InvoiceResponse, error) {
	customerID := types.GetCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	invoiceService := NewInvoiceService(s.ServiceParams)

	// Get the invoice with breakdown
	req := dto.GetInvoiceWithBreakdownRequest{
		ID: invoiceID,
	}
	invoice, err := invoiceService.GetInvoiceWithBreakdown(ctx, req)
	if err != nil {
		return nil, err
	}

	// Verify it belongs to this customer
	if invoice.CustomerID != customerID {
		return nil, ierr.NewError("invoice not found").
			WithHint("Invoice does not belong to this customer").
			Mark(ierr.ErrNotFound)
	}

	return invoice, nil
}

// GetWallets returns wallet balances for the dashboard customer
func (s *customerDashboardService) GetWallets(ctx context.Context) ([]*dto.WalletBalanceResponse, error) {
	customerID := types.GetCustomerID(ctx)
	externalCustomerID := types.GetExternalCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	walletService := NewWalletService(s.ServiceParams)

	getWalletsReq := &dto.GetCustomerWalletsRequest{
		ID:        customerID,
		LookupKey: externalCustomerID,
	}
	return walletService.GetCustomerWallets(ctx, getWalletsReq)
}

// GetWallet returns a specific wallet
func (s *customerDashboardService) GetWallet(ctx context.Context, walletID string) (*dto.WalletBalanceResponse, error) {
	customerID := types.GetCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	walletService := NewWalletService(s.ServiceParams)

	// Get the wallet balance
	wallet, err := walletService.GetWalletBalance(ctx, walletID)
	if err != nil {
		return nil, err
	}

	// Verify it belongs to this customer
	if wallet.CustomerID != customerID {
		return nil, ierr.NewError("wallet not found").
			WithHint("Wallet does not belong to this customer").
			Mark(ierr.ErrNotFound)
	}

	return wallet, nil
}

// GetAnalytics returns usage analytics for the dashboard customer
func (s *customerDashboardService) GetAnalytics(ctx context.Context, req dto.DashboardAnalyticsRequest) (*dto.GetUsageAnalyticsResponse, error) {
	customerID := types.GetCustomerID(ctx)
	externalCustomerID := types.GetExternalCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	// Convert dashboard request to internal request with customer ID injected
	internalReq := req.ToInternalRequest(externalCustomerID)

	// Use FeatureUsageTrackingService to get detailed usage analytics
	featureUsageTrackingService := NewFeatureUsageTrackingService(s.ServiceParams, s.EventRepo, s.FeatureUsageRepo)
	return featureUsageTrackingService.GetDetailedUsageAnalyticsV2(ctx, internalReq)
}

// GetCostAnalytics returns cost analytics for the dashboard customer
func (s *customerDashboardService) GetCostAnalytics(ctx context.Context, req dto.DashboardCostAnalyticsRequest) (*dto.GetDetailedCostAnalyticsResponse, error) {
	customerID := types.GetCustomerID(ctx)
	externalCustomerID := types.GetExternalCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	// Convert dashboard request to internal request with customer ID injected
	internalReq := req.ToInternalRequest(externalCustomerID)

	// Validate internal request
	if err := internalReq.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid cost analytics request").
			Mark(ierr.ErrValidation)
	}

	// For now, return only revenue analytics (usage analytics)
	// Full cost analytics requires CostSheetUsageRepository which is not available in ServiceParams
	revenueReq := &dto.GetUsageAnalyticsRequest{
		ExternalCustomerID: externalCustomerID,
		FeatureIDs:         internalReq.FeatureIDs,
		StartTime:          internalReq.StartTime,
		EndTime:            internalReq.EndTime,
	}

	featureUsageTrackingService := NewFeatureUsageTrackingService(s.ServiceParams, s.EventRepo, s.FeatureUsageRepo)
	revenueAnalytics, err := featureUsageTrackingService.GetDetailedUsageAnalyticsV2(ctx, revenueReq)
	if err != nil {
		return nil, err
	}

	// Build response with revenue data only
	response := &dto.GetDetailedCostAnalyticsResponse{
		CostAnalytics: []dto.CostAnalyticItem{},
		TotalCost:     decimal.Zero,
		TotalRevenue:  revenueAnalytics.TotalCost, // TotalCost in usage analytics represents revenue
		Margin:        revenueAnalytics.TotalCost, // Without cost data, margin equals revenue
		MarginPercent: decimal.NewFromInt(100),    // 100% margin without cost data
		ROI:           decimal.Zero,
		ROIPercent:    decimal.Zero,
		Currency:      revenueAnalytics.Currency,
		StartTime:     internalReq.StartTime,
		EndTime:       internalReq.EndTime,
	}

	return response, nil
}
