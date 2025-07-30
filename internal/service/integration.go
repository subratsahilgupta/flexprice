package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/connection"
	"github.com/flexprice/flexprice/internal/domain/customer"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/stripe/stripe-go/v79"
)

// IntegrationService handles generic integration operations with multiple providers
type IntegrationService interface {
	// SyncCustomerToProviders syncs a customer to all available providers for the tenant
	SyncCustomerToProviders(ctx context.Context, customerID string) error

	// SyncCustomerFromProvider syncs a customer from a specific provider to FlexPrice
	SyncCustomerFromProvider(ctx context.Context, providerType string, providerCustomerID string, customerData map[string]interface{}) error

	// GetAvailableProviders returns all available providers for the current tenant
	GetAvailableProviders(ctx context.Context) ([]*connection.Connection, error)
}

type integrationService struct {
	ServiceParams
}

func NewIntegrationService(params ServiceParams) IntegrationService {
	return &integrationService{
		ServiceParams: params,
	}
}

// SyncCustomerToProviders syncs a customer to all available providers for the tenant
func (s *integrationService) SyncCustomerToProviders(ctx context.Context, customerID string) error {
	// Get the customer
	customerService := NewCustomerService(s.ServiceParams)
	customerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return err
	}
	customer := customerResp.Customer

	// Get all available connections for this tenant
	connections, err := s.getAvailableConnections(ctx)
	if err != nil {
		return err
	}

	if len(connections) == 0 {
		s.Logger.Infow("no integrations available for customer sync",
			"customer_id", customerID,
			"tenant_id", types.GetTenantID(ctx))
		return nil
	}

	// Sync to each provider
	for _, conn := range connections {
		// Use goroutine to avoid blocking
		go func(connection *connection.Connection) {
			syncCtx := types.SetTenantID(context.Background(), types.GetTenantID(ctx))
			syncCtx = types.SetEnvironmentID(syncCtx, types.GetEnvironmentID(ctx))
			syncCtx = types.SetUserID(syncCtx, types.GetUserID(ctx))

			if err := s.syncCustomerToProvider(syncCtx, customer, connection); err != nil {
				s.Logger.Errorw("failed to sync customer to provider",
					"customer_id", customerID,
					"provider_type", connection.ProviderType,
					"error", err)
			} else {
				s.Logger.Infow("customer synced to provider successfully",
					"customer_id", customerID,
					"provider_type", connection.ProviderType)
			}
		}(conn)
	}

	return nil
}

// syncCustomerToProvider syncs a customer to a specific provider
func (s *integrationService) syncCustomerToProvider(ctx context.Context, customer *customer.Customer, conn *connection.Connection) error {
	// Check if mapping already exists for this customer_id, provider, tenant, and environment
	entityMappingService := NewEntityIntegrationMappingService(s.ServiceParams)
	existingMapping, err := entityMappingService.GetByEntityAndProvider(
		ctx, customer.ID, "customer", string(conn.ProviderType))

	if err == nil && existingMapping != nil {
		// Mapping exists, customer already synced
		s.Logger.Infow("customer already mapped to provider",
			"customer_id", customer.ID,
			"provider_type", conn.ProviderType,
			"provider_entity_id", existingMapping.ProviderEntityID)
		return nil
	}

	// Sync based on provider type
	var providerEntityID string
	var metadata map[string]interface{}

	switch conn.ProviderType {
	case types.SecretProviderStripe:
		providerEntityID, metadata, err = s.syncCustomerToStripe(ctx, customer, conn)
	// Add more providers as needed
	default:
		return ierr.NewError("unsupported provider type").
			WithHint(fmt.Sprintf("Provider type %s is not supported", conn.ProviderType)).
			Mark(ierr.ErrValidation)
	}

	if err != nil {
		return err
	}

	// Create entity mapping
	mappingReq := dto.CreateEntityIntegrationMappingRequest{
		EntityID:         customer.ID,
		EntityType:       "customer",
		ProviderType:     string(conn.ProviderType),
		ProviderEntityID: providerEntityID,
		Metadata:         metadata,
	}

	_, err = entityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
	if err != nil {
		s.Logger.Errorw("failed to create entity mapping",
			"customer_id", customer.ID,
			"provider_type", conn.ProviderType,
			"provider_entity_id", providerEntityID,
			"error", err)
		return err
	}

	// Update customer metadata with provider ID
	updateReq := dto.UpdateCustomerRequest{
		Metadata: map[string]string{
			fmt.Sprintf("%s_customer_id", conn.ProviderType): providerEntityID,
		},
	}

	// Merge with existing metadata
	if customer.Metadata != nil {
		for k, v := range customer.Metadata {
			updateReq.Metadata[k] = v
		}
	}

	customerService := NewCustomerService(s.ServiceParams)
	_, err = customerService.UpdateCustomer(ctx, customer.ID, updateReq)
	if err != nil {
		s.Logger.Errorw("failed to update customer metadata",
			"customer_id", customer.ID,
			"provider_type", conn.ProviderType,
			"error", err)
		return err
	}

	return nil
}

// syncCustomerToStripe syncs a customer to Stripe
func (s *integrationService) syncCustomerToStripe(ctx context.Context, customer *customer.Customer, conn *connection.Connection) (string, map[string]interface{}, error) {
	stripeService := NewStripeService(s.ServiceParams)

	// Get Stripe configuration
	stripeConfig, err := stripeService.GetDecryptedStripeConfig(conn)
	if err != nil {
		return "", nil, err
	}

	// Check if customer already has Stripe ID
	if stripeID, exists := customer.Metadata["stripe_customer_id"]; exists && stripeID != "" {
		return stripeID, map[string]interface{}{
			"stripe_customer_email": customer.Email,
			"sync_direction":        "flexprice_to_provider",
			"created_via":           "api",
		}, nil
	}

	// Check if customer with same email already exists in Stripe
	if customer.Email != "" {
		existingStripeCustomer, err := s.findStripeCustomerByEmail(ctx, stripeConfig.SecretKey, customer.Email)
		if err == nil && existingStripeCustomer != nil {
			s.Logger.Infow("customer with same email already exists in Stripe",
				"customer_id", customer.ID,
				"email", customer.Email,
				"stripe_customer_id", existingStripeCustomer.ID)
			return existingStripeCustomer.ID, map[string]interface{}{
				"stripe_customer_email": customer.Email,
				"sync_direction":        "flexprice_to_provider",
				"created_via":           "api",
				"found_existing":        true,
			}, nil
		}
	}

	// Create customer in Stripe
	if err := stripeService.CreateCustomerInStripe(ctx, customer.ID); err != nil {
		return "", nil, err
	}

	// Get updated customer to get the Stripe ID
	customerService := NewCustomerService(s.ServiceParams)
	customerResp, err := customerService.GetCustomer(ctx, customer.ID)
	if err != nil {
		return "", nil, err
	}

	stripeID := customerResp.Customer.Metadata["stripe_customer_id"]
	if stripeID == "" {
		return "", nil, ierr.NewError("failed to get Stripe customer ID").
			WithHint("Stripe customer ID not found in metadata").
			Mark(ierr.ErrInternal)
	}

	return stripeID, map[string]interface{}{
		"stripe_customer_email": customer.Email,
		"stripe_customer_name":  customer.Name,
		"sync_direction":        "flexprice_to_provider",
		"created_via":           "api",
		"synced_at":             time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// SyncCustomerFromProvider syncs a customer from a specific provider to FlexPrice
func (s *integrationService) SyncCustomerFromProvider(ctx context.Context, providerType string, providerCustomerID string, customerData map[string]interface{}) error {
	// Check if mapping already exists
	entityMappingService := NewEntityIntegrationMappingService(s.ServiceParams)
	existingMapping, err := entityMappingService.GetByProviderEntity(ctx, providerType, providerCustomerID)

	if err == nil && existingMapping != nil {
		// Mapping exists, customer already synced
		s.Logger.Infow("customer already exists from provider",
			"provider_type", providerType,
			"provider_customer_id", providerCustomerID,
			"flexprice_customer_id", existingMapping.EntityID)
		return nil
	}

	// Check for existing customer by email in FlexPrice
	if email, exists := customerData["email"].(string); exists && email != "" {
		customerService := NewCustomerService(s.ServiceParams)
		existingCustomer, err := s.findCustomerByEmail(ctx, email)
		if err == nil && existingCustomer != nil {
			// Customer with same email exists, update with provider ID
			s.Logger.Infow("customer with same email already exists in FlexPrice",
				"email", email,
				"flexprice_customer_id", existingCustomer.ID,
				"provider_type", providerType,
				"provider_customer_id", providerCustomerID)

			// Update existing customer with provider ID
			updateReq := dto.UpdateCustomerRequest{
				Metadata: map[string]string{
					fmt.Sprintf("%s_customer_id", providerType): providerCustomerID,
				},
			}

			// Merge with existing metadata
			if existingCustomer.Metadata != nil {
				for k, v := range existingCustomer.Metadata {
					updateReq.Metadata[k] = v
				}
			}

			_, err = customerService.UpdateCustomer(ctx, existingCustomer.ID, updateReq)
			if err != nil {
				return err
			}

			// Create entity mapping for existing customer
			mappingReq := dto.CreateEntityIntegrationMappingRequest{
				EntityID:         existingCustomer.ID,
				EntityType:       "customer",
				ProviderType:     providerType,
				ProviderEntityID: providerCustomerID,
				Metadata: map[string]interface{}{
					"sync_direction": "provider_to_flexprice",
					"created_via":    "webhook",
					"found_existing": true,
				},
			}

			_, err = entityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
			return err
		}
	}

	// Create customer based on provider type
	var customerID string
	var metadata map[string]interface{}

	switch providerType {
	case "stripe":
		customerID, metadata, err = s.createCustomerFromStripe(ctx, providerCustomerID, customerData)
	default:
		return ierr.NewError("unsupported provider type").
			WithHint(fmt.Sprintf("Provider type %s is not supported", providerType)).
			Mark(ierr.ErrValidation)
	}

	if err != nil {
		return err
	}

	// Create entity mapping
	mappingReq := dto.CreateEntityIntegrationMappingRequest{
		EntityID:         customerID,
		EntityType:       "customer",
		ProviderType:     providerType,
		ProviderEntityID: providerCustomerID,
		Metadata:         metadata,
	}

	_, err = entityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
	if err != nil {
		s.Logger.Errorw("failed to create entity mapping from provider",
			"customer_id", customerID,
			"provider_type", providerType,
			"provider_customer_id", providerCustomerID,
			"error", err)
		return err
	}

	return nil
}

// createCustomerFromStripe creates a customer in FlexPrice from Stripe webhook data
func (s *integrationService) createCustomerFromStripe(ctx context.Context, stripeCustomerID string, customerData map[string]interface{}) (string, map[string]interface{}, error) {
	// Convert customerData to Stripe customer format for the Stripe service
	stripeCustomer := &stripe.Customer{
		ID:       stripeCustomerID,
		Name:     customerData["name"].(string),
		Email:    customerData["email"].(string),
		Metadata: map[string]string{},
	}

	// Add flexprice_customer_id to metadata if it exists
	if flexpriceID, exists := customerData["flexprice_customer_id"]; exists && flexpriceID != nil {
		if flexpriceIDStr, ok := flexpriceID.(string); ok {
			stripeCustomer.Metadata["flexprice_customer_id"] = flexpriceIDStr
		}
	}

	// Add address if available
	if address, exists := customerData["address"]; exists && address != nil {
		if addrMap, ok := address.(map[string]interface{}); ok {
			stripeCustomer.Address = &stripe.Address{}

			if line1, exists := addrMap["line1"]; exists && line1 != nil {
				if line1Str, ok := line1.(string); ok {
					stripeCustomer.Address.Line1 = line1Str
				}
			}
			if line2, exists := addrMap["line2"]; exists && line2 != nil {
				if line2Str, ok := line2.(string); ok {
					stripeCustomer.Address.Line2 = line2Str
				}
			}
			if city, exists := addrMap["city"]; exists && city != nil {
				if cityStr, ok := city.(string); ok {
					stripeCustomer.Address.City = cityStr
				}
			}
			if state, exists := addrMap["state"]; exists && state != nil {
				if stateStr, ok := state.(string); ok {
					stripeCustomer.Address.State = stateStr
				}
			}
			if postalCode, exists := addrMap["postal_code"]; exists && postalCode != nil {
				if postalCodeStr, ok := postalCode.(string); ok {
					stripeCustomer.Address.PostalCode = postalCodeStr
				}
			}
			if country, exists := addrMap["country"]; exists && country != nil {
				if countryStr, ok := country.(string); ok {
					stripeCustomer.Address.Country = countryStr
				}
			}
		}
	}

	// Use the Stripe service to create customer
	stripeService := NewStripeService(s.ServiceParams)
	environmentID := types.GetEnvironmentID(ctx)
	if err := stripeService.CreateCustomerFromStripe(ctx, stripeCustomer, environmentID); err != nil {
		return "", nil, err
	}

	// Get the created customer
	customerService := NewCustomerService(s.ServiceParams)
	customerResp, err := customerService.GetCustomerByLookupKey(ctx, stripeCustomerID)
	if err != nil {
		return "", nil, err
	}

	return customerResp.Customer.ID, map[string]interface{}{
		"stripe_customer_email": customerData["email"].(string),
		"stripe_customer_name":  customerData["name"].(string),
		"sync_direction":        "provider_to_flexprice",
		"created_via":           "webhook",
		"webhook_event":         "customer.created",
		"synced_at":             time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// getAvailableConnections gets all available connections for the current tenant
func (s *integrationService) getAvailableConnections(ctx context.Context) ([]*connection.Connection, error) {
	if s.ConnectionRepo == nil {
		return nil, nil
	}

	filter := types.NewConnectionFilter()
	filter.Limit = lo.ToPtr(100) // Get up to 100 connections

	connections, err := s.ConnectionRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Filter out non-provider connections (like flexprice)
	var providerConnections []*connection.Connection
	for _, conn := range connections {
		if conn.ProviderType != types.SecretProviderFlexPrice {
			providerConnections = append(providerConnections, conn)
		}
	}

	return providerConnections, nil
}

// GetAvailableProviders returns all available providers for the current tenant
func (s *integrationService) GetAvailableProviders(ctx context.Context) ([]*connection.Connection, error) {
	return s.getAvailableConnections(ctx)
}

// findStripeCustomerByEmail finds a customer in Stripe by email
func (s *integrationService) findStripeCustomerByEmail(ctx context.Context, secretKey, email string) (*stripe.Customer, error) {
	// For now, we'll skip this check as it requires additional Stripe API calls
	// In a production environment, you might want to implement this using Stripe's search API
	// or maintain a local cache of email to customer ID mappings

	// TODO: Implement proper Stripe customer search by email
	// This would require using Stripe's search API or customer list API
	// For now, we'll return nil to indicate no existing customer found
	return nil, nil
}

// findCustomerByEmail finds a customer by email in FlexPrice
func (s *integrationService) findCustomerByEmail(ctx context.Context, email string) (*customer.Customer, error) {
	customerService := NewCustomerService(s.ServiceParams)

	// Use customer filter to search by email
	filter := &types.CustomerFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
	}

	customers, err := customerService.GetCustomers(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Search through customers for matching email
	for _, customer := range customers.Items {
		if customer.Customer.Email == email {
			return customer.Customer, nil
		}
	}

	return nil, nil // No customer found
}
