package service

import (
	"context"
	"encoding/json"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/client"
)

// StripeService handles Stripe integration operations
type StripeService struct {
	ServiceParams
}

// NewStripeService creates a new Stripe service instance
func NewStripeService(params ServiceParams) *StripeService {
	return &StripeService{
		ServiceParams: params,
	}
}

// CreateCustomerInStripe creates a customer in Stripe and updates our customer with Stripe ID
func (s *StripeService) CreateCustomerInStripe(ctx context.Context, customerID string) error {
	// Get our customer
	customerService := NewCustomerService(s.ServiceParams)
	ourCustomerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return err
	}
	ourCustomer := ourCustomerResp.Customer

	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByEnvironmentAndProvider(ctx, ourCustomer.EnvironmentID, types.SecretProviderStripe)
	if err != nil {
		return ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	stripeConfig, err := conn.GetStripeConfig()
	if err != nil {
		return ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	sc := &client.API{}
	sc.Init(stripeConfig.SecretKey, nil)

	// Check if customer already has Stripe ID
	if stripeID, exists := ourCustomer.Metadata["stripe_customer_id"]; exists && stripeID != "" {
		return ierr.NewError("customer already has Stripe ID").
			WithHint("Customer is already synced with Stripe").
			Mark(ierr.ErrAlreadyExists)
	}

	// Create customer in Stripe
	params := &stripe.CustomerParams{
		Name:  stripe.String(ourCustomer.Name),
		Email: stripe.String(ourCustomer.Email),
		Metadata: map[string]string{
			"flexprice_customer_id": ourCustomer.ID,
			"flexprice_environment": ourCustomer.EnvironmentID,
		},
	}

	// Add address if available
	if ourCustomer.AddressLine1 != "" || ourCustomer.AddressCity != "" {
		params.Address = &stripe.AddressParams{
			Line1:      stripe.String(ourCustomer.AddressLine1),
			Line2:      stripe.String(ourCustomer.AddressLine2),
			City:       stripe.String(ourCustomer.AddressCity),
			State:      stripe.String(ourCustomer.AddressState),
			PostalCode: stripe.String(ourCustomer.AddressPostalCode),
			Country:    stripe.String(ourCustomer.AddressCountry),
		}
	}

	stripeCustomer, err := sc.Customers.New(params)
	if err != nil {
		return ierr.NewError("failed to create customer in Stripe").
			WithHint("Stripe API error").
			Mark(ierr.ErrHTTPClient)
	}

	// Update our customer with Stripe ID
	updateReq := dto.UpdateCustomerRequest{
		Metadata: map[string]string{
			"stripe_customer_id": stripeCustomer.ID,
		},
	}
	// Merge with existing metadata
	if ourCustomer.Metadata != nil {
		for k, v := range ourCustomer.Metadata {
			updateReq.Metadata[k] = v
		}
	}

	_, err = customerService.UpdateCustomer(ctx, ourCustomer.ID, updateReq)
	if err != nil {
		return err
	}

	return nil
}

// SyncCustomerFromStripe creates or updates our customer from Stripe webhook data
func (s *StripeService) SyncCustomerFromStripe(ctx context.Context, stripeCustomer *stripe.Customer, environmentID string) error {
	// Check if customer already exists by Stripe ID
	existingCustomer, err := s.findCustomerByStripeID(ctx, environmentID, stripeCustomer.ID)
	if err == nil && existingCustomer != nil {
		// Customer exists, update if needed
		return s.updateExistingCustomer(ctx, existingCustomer, stripeCustomer)
	}

	// Customer doesn't exist, create new one
	return s.createCustomerFromStripe(ctx, stripeCustomer, environmentID)
}

func (s *StripeService) createCustomerFromStripe(ctx context.Context, stripeCustomer *stripe.Customer, environmentID string) error {
	// Create customer service instance
	customerService := NewCustomerService(s.ServiceParams)

	// Check for duplicate by external ID if present
	var externalID string
	if flexpriceID, exists := stripeCustomer.Metadata["flexprice_customer_id"]; exists {
		externalID = flexpriceID
		// Check if customer with this external ID already exists
		existing, err := s.findCustomerByExternalID(ctx, environmentID, externalID)
		if err == nil && existing != nil {
			// Customer exists with this external ID, update with Stripe ID
			updateReq := dto.UpdateCustomerRequest{
				Metadata: map[string]string{
					"stripe_customer_id": stripeCustomer.ID,
				},
			}
			// Merge with existing metadata
			if existing.Metadata != nil {
				for k, v := range existing.Metadata {
					updateReq.Metadata[k] = v
				}
			}
			_, err = customerService.UpdateCustomer(ctx, existing.ID, updateReq)
			return err
		}
	} else {
		// Generate external ID if not present
		externalID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CUSTOMER)
	}

	// Create new customer using DTO
	createReq := dto.CreateCustomerRequest{
		ExternalID: externalID,
		Name:       stripeCustomer.Name,
		Email:      stripeCustomer.Email,
		Metadata: map[string]string{
			"stripe_customer_id": stripeCustomer.ID,
		},
	}

	// Add address if available
	if stripeCustomer.Address != nil {
		createReq.AddressLine1 = stripeCustomer.Address.Line1
		createReq.AddressLine2 = stripeCustomer.Address.Line2
		createReq.AddressCity = stripeCustomer.Address.City
		createReq.AddressState = stripeCustomer.Address.State
		createReq.AddressPostalCode = stripeCustomer.Address.PostalCode
		createReq.AddressCountry = stripeCustomer.Address.Country
	}

	_, err := customerService.CreateCustomer(ctx, createReq)
	return err
}

func (s *StripeService) updateExistingCustomer(ctx context.Context, existingCustomer *dto.CustomerResponse, stripeCustomer *stripe.Customer) error {
	// Create customer service instance
	customerService := NewCustomerService(s.ServiceParams)

	updateReq := dto.UpdateCustomerRequest{}
	needsUpdate := false

	// Update name if different
	if stripeCustomer.Name != existingCustomer.Name {
		updateReq.Name = &stripeCustomer.Name
		needsUpdate = true
	}

	// Update email if different
	if stripeCustomer.Email != existingCustomer.Email {
		updateReq.Email = &stripeCustomer.Email
		needsUpdate = true
	}

	// Update address if different
	if stripeCustomer.Address != nil {
		if stripeCustomer.Address.Line1 != existingCustomer.AddressLine1 {
			updateReq.AddressLine1 = &stripeCustomer.Address.Line1
			needsUpdate = true
		}
		if stripeCustomer.Address.Line2 != existingCustomer.AddressLine2 {
			updateReq.AddressLine2 = &stripeCustomer.Address.Line2
			needsUpdate = true
		}
		if stripeCustomer.Address.City != existingCustomer.AddressCity {
			updateReq.AddressCity = &stripeCustomer.Address.City
			needsUpdate = true
		}
		if stripeCustomer.Address.State != existingCustomer.AddressState {
			updateReq.AddressState = &stripeCustomer.Address.State
			needsUpdate = true
		}
		if stripeCustomer.Address.PostalCode != existingCustomer.AddressPostalCode {
			updateReq.AddressPostalCode = &stripeCustomer.Address.PostalCode
			needsUpdate = true
		}
		if stripeCustomer.Address.Country != existingCustomer.AddressCountry {
			updateReq.AddressCountry = &stripeCustomer.Address.Country
			needsUpdate = true
		}
	}

	if needsUpdate {
		_, err := customerService.UpdateCustomer(ctx, existingCustomer.ID, updateReq)
		return err
	}

	return nil
}

// ParseWebhookEvent parses a Stripe webhook event
func (s *StripeService) ParseWebhookEvent(payload []byte) (*stripe.Event, error) {
	var event stripe.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, ierr.NewError("failed to parse webhook event").
			WithHint("Invalid webhook payload").
			Mark(ierr.ErrValidation)
	}
	return &event, nil
}

// findCustomerByStripeID finds a customer by Stripe customer ID in metadata
func (s *StripeService) findCustomerByStripeID(ctx context.Context, environmentID, stripeCustomerID string) (*dto.CustomerResponse, error) {
	// Use customer filter to search by metadata
	filter := &types.CustomerFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
	}

	customerService := NewCustomerService(s.ServiceParams)
	customers, err := customerService.GetCustomers(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Search through customers for matching Stripe ID
	for _, customer := range customers.Items {
		if customer.Metadata != nil {
			if stripeID, exists := customer.Metadata["stripe_customer_id"]; exists && stripeID == stripeCustomerID {
				return customer, nil
			}
		}
	}

	return nil, ierr.NewError("customer not found").Mark(ierr.ErrNotFound)
}

// findCustomerByExternalID finds a customer by external ID
func (s *StripeService) findCustomerByExternalID(ctx context.Context, environmentID, externalID string) (*dto.CustomerResponse, error) {
	// Use customer filter to search by external ID
	filter := &types.CustomerFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
	}

	customerService := NewCustomerService(s.ServiceParams)
	customers, err := customerService.GetCustomers(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Search through customers for matching external ID
	for _, customer := range customers.Items {
		if customer.ExternalID == externalID {
			return customer, nil
		}
	}

	return nil, ierr.NewError("customer not found").Mark(ierr.ErrNotFound)
}
