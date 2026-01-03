package service

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/api/dto"
	fpauth "github.com/flexprice/flexprice/internal/auth"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
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
	GetSubscriptions(ctx context.Context) (*dto.ListSubscriptionsResponse, error)
	// GetInvoices returns invoices for the dashboard customer
	GetInvoices(ctx context.Context) (*dto.ListInvoicesResponse, error)
}

type customerDashboardService struct {
	ServiceParams
	authProvider        fpauth.Provider
	customerService     CustomerService
	subscriptionService SubscriptionService
	invoiceService      InvoiceService
	walletService       WalletService
	billingService      BillingService
}

// NewCustomerDashboardService creates a new customer dashboard service
func NewCustomerDashboardService(
	params ServiceParams,
	authProvider fpauth.Provider,
	customerService CustomerService,
	subscriptionService SubscriptionService,
	invoiceService InvoiceService,
	walletService WalletService,
	billingService BillingService,
) CustomerDashboardService {
	return &customerDashboardService{
		ServiceParams:       params,
		authProvider:        authProvider,
		customerService:     customerService,
		subscriptionService: subscriptionService,
		invoiceService:      invoiceService,
		walletService:       walletService,
		billingService:      billingService,
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

	token, expiresAt, err := s.authProvider.GenerateDashboardToken(customer.ID, req.ExternalCustomerID, tenantID, environmentID)
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
func (s *customerDashboardService) GetSubscriptions(ctx context.Context) (*dto.ListSubscriptionsResponse, error) {
	customerID := types.GetCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	filter := &types.SubscriptionFilter{
		CustomerID:  customerID,
		QueryFilter: types.NewDefaultQueryFilter(),
	}

	return s.subscriptionService.ListSubscriptions(ctx, filter)
}

// GetInvoices returns invoices for the dashboard customer
func (s *customerDashboardService) GetInvoices(ctx context.Context) (*dto.ListInvoicesResponse, error) {
	customerID := types.GetCustomerID(ctx)
	if customerID == "" {
		return nil, ierr.NewError("customer not found in context").Mark(ierr.ErrPermissionDenied)
	}

	filter := &types.InvoiceFilter{
		CustomerID:  customerID,
		QueryFilter: types.NewDefaultQueryFilter(),
	}

	return s.invoiceService.ListInvoices(ctx, filter)
}
