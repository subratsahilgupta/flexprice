# Stripe Customer Sync with Entity Integration Mapping

This example demonstrates how to use the Entity Integration Mapping system to handle bidirectional customer synchronization between FlexPrice and Stripe.

## Overview

The example shows:
1. **FlexPrice → Stripe**: Creating a customer on FlexPrice and syncing to Stripe
2. **Stripe → FlexPrice**: Handling Stripe webhooks to create customers on FlexPrice
3. **Duplicate Prevention**: Using mappings to prevent duplicate entities
4. **Error Handling**: Proper error handling for various scenarios

## Implementation

### 1. FlexPrice → Stripe Customer Sync

```go
package service

import (
    "context"
    "errors"
    
    "github.com/flexprice/flexprice/internal/api/dto"
    "github.com/flexprice/flexprice/internal/types"
    ierr "github.com/flexprice/flexprice/internal/errors"
)

// CustomerServiceWithMapping extends the customer service with mapping capabilities
type CustomerServiceWithMapping struct {
    *CustomerService
    EntityMappingService EntityIntegrationMappingService
    StripeService       StripeService
}

// CreateCustomerWithStripeSync creates a customer on FlexPrice and syncs to Stripe
func (s *CustomerServiceWithMapping) CreateCustomerWithStripeSync(
    ctx context.Context, 
    req dto.CreateCustomerRequest,
) (*dto.CustomerResponse, error) {
    // 1. Create customer on FlexPrice
    customer, err := s.CreateCustomer(ctx, req)
    if err != nil {
        return nil, err
    }

    // 2. Check if mapping already exists
    existingMapping, err := s.EntityMappingService.GetByEntityAndProvider(
        ctx, customer.ID, "customer", "stripe")
    
    if existingMapping != nil {
        // Mapping exists, return existing Stripe customer ID
        s.Logger.Infow("customer already mapped to Stripe",
            "customer_id", customer.ID,
            "stripe_customer_id", existingMapping.ProviderEntityID)
        return customer, nil
    }

    // 3. Create customer on Stripe
    stripeReq := dto.CreateStripeCustomerRequest{
        Email:     customer.Email,
        Name:      customer.Name,
        Metadata: map[string]string{
            "flexprice_customer_id": customer.ID,
        },
    }
    
    stripeCustomer, err := s.StripeService.CreateCustomer(ctx, stripeReq)
    if err != nil {
        s.Logger.Errorw("failed to create Stripe customer",
            "error", err,
            "customer_id", customer.ID)
        return nil, ierr.NewError("failed to sync customer to Stripe").Wrap(err)
    }

    // 4. Create mapping
    mappingReq := dto.CreateEntityIntegrationMappingRequest{
        EntityID:         customer.ID,
        EntityType:       "customer",
        ProviderType:     "stripe",
        ProviderEntityID: stripeCustomer.ID,
        Metadata: map[string]interface{}{
            "stripe_customer_email": customer.Email,
            "stripe_customer_name":  customer.Name,
            "sync_direction":        "flexprice_to_provider",
            "created_via":           "api",
        },
    }
    
    mapping, err := s.EntityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
    if err != nil {
        s.Logger.Errorw("failed to create entity mapping",
            "error", err,
            "customer_id", customer.ID,
            "stripe_customer_id", stripeCustomer.ID)
        // Note: We don't fail here as the customer was created successfully
        // The mapping can be retried later
    }

    s.Logger.Infow("customer created and synced to Stripe",
        "customer_id", customer.ID,
        "stripe_customer_id", stripeCustomer.ID,
        "mapping_id", mapping.ID)

    return customer, nil
}
```

### 2. Stripe Webhook Handler

```go
package webhook

import (
    "context"
    "errors"
    
    "github.com/flexprice/flexprice/internal/api/dto"
    "github.com/flexprice/flexprice/internal/types"
    ierr "github.com/flexprice/flexprice/internal/errors"
    "github.com/stripe/stripe-go/v74"
)

// StripeWebhookHandler handles Stripe webhook events
type StripeWebhookHandler struct {
    CustomerService       CustomerService
    EntityMappingService  EntityIntegrationMappingService
    Logger               *logger.Logger
}

// HandleCustomerCreated handles Stripe customer.created webhook
func (h *StripeWebhookHandler) HandleCustomerCreated(
    ctx context.Context,
    event *stripe.Event,
) error {
    customer := event.Data.Object.(*stripe.Customer)
    
    h.Logger.Infow("handling Stripe customer.created webhook",
        "stripe_customer_id", customer.ID,
        "stripe_customer_email", customer.Email)

    // 1. Check if mapping exists
    existingMapping, err := h.EntityMappingService.GetByProviderEntity(
        ctx, "stripe", customer.ID)
    
    if err == nil && existingMapping != nil {
        // Mapping exists, customer already synced
        h.Logger.Infow("customer already exists on FlexPrice",
            "stripe_customer_id", customer.ID,
            "flexprice_customer_id", existingMapping.EntityID)
        return nil
    }

    if err != nil && !errors.Is(err, types.ErrNotFound) {
        h.Logger.Errorw("failed to check existing mapping",
            "error", err,
            "stripe_customer_id", customer.ID)
        return ierr.NewError("failed to check existing mapping").Wrap(err)
    }

    // 2. Create customer on FlexPrice
    customerReq := dto.CreateCustomerRequest{
        Name:  customer.Name,
        Email: customer.Email,
        Metadata: map[string]interface{}{
            "stripe_customer_id": customer.ID,
            "created_via":        "webhook",
            "webhook_event":      "customer.created",
        },
    }
    
    flexpriceCustomer, err := h.CustomerService.CreateCustomer(ctx, customerReq)
    if err != nil {
        h.Logger.Errorw("failed to create FlexPrice customer from webhook",
            "error", err,
            "stripe_customer_id", customer.ID)
        return ierr.NewError("failed to create customer from webhook").Wrap(err)
    }

    // 3. Create mapping
    mappingReq := dto.CreateEntityIntegrationMappingRequest{
        EntityID:         flexpriceCustomer.ID,
        EntityType:       "customer",
        ProviderType:     "stripe",
        ProviderEntityID: customer.ID,
        Metadata: map[string]interface{}{
            "stripe_customer_email": customer.Email,
            "stripe_customer_name":  customer.Name,
            "sync_direction":        "provider_to_flexprice",
            "webhook_event":         "customer.created",
            "webhook_id":            event.ID,
        },
    }
    
    mapping, err := h.EntityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
    if err != nil {
        h.Logger.Errorw("failed to create entity mapping from webhook",
            "error", err,
            "stripe_customer_id", customer.ID,
            "flexprice_customer_id", flexpriceCustomer.ID)
        // Note: We don't fail here as the customer was created successfully
        // The mapping can be retried later
    }

    h.Logger.Infow("customer created from Stripe webhook",
        "stripe_customer_id", customer.ID,
        "flexprice_customer_id", flexpriceCustomer.ID,
        "mapping_id", mapping.ID)

    return nil
}

// HandleCustomerUpdated handles Stripe customer.updated webhook
func (h *StripeWebhookHandler) HandleCustomerUpdated(
    ctx context.Context,
    event *stripe.Event,
) error {
    customer := event.Data.Object.(*stripe.Customer)
    
    h.Logger.Infow("handling Stripe customer.updated webhook",
        "stripe_customer_id", customer.ID)

    // 1. Find existing mapping
    mapping, err := h.EntityMappingService.GetByProviderEntity(
        ctx, "stripe", customer.ID)
    
    if err != nil {
        if errors.Is(err, types.ErrNotFound) {
            h.Logger.Warnw("customer updated but no mapping found",
                "stripe_customer_id", customer.ID)
            return nil // Not an error, just log
        }
        return ierr.NewError("failed to find customer mapping").Wrap(err)
    }

    // 2. Update FlexPrice customer
    updateReq := dto.UpdateCustomerRequest{
        Name:  &customer.Name,
        Email: &customer.Email,
        Metadata: map[string]interface{}{
            "stripe_customer_id": customer.ID,
            "updated_via":        "webhook",
            "webhook_event":      "customer.updated",
        },
    }
    
    _, err = h.CustomerService.UpdateCustomer(ctx, mapping.EntityID, updateReq)
    if err != nil {
        h.Logger.Errorw("failed to update FlexPrice customer from webhook",
            "error", err,
            "stripe_customer_id", customer.ID,
            "flexprice_customer_id", mapping.EntityID)
        return ierr.NewError("failed to update customer from webhook").Wrap(err)
    }

    // 3. Update mapping metadata
    updateMappingReq := dto.UpdateEntityIntegrationMappingRequest{
        Metadata: map[string]interface{}{
            "stripe_customer_email": customer.Email,
            "stripe_customer_name":  customer.Name,
            "last_webhook_event":    "customer.updated",
            "last_webhook_id":       event.ID,
            "updated_at":            event.Created,
        },
    }
    
    _, err = h.EntityMappingService.UpdateEntityIntegrationMapping(
        ctx, mapping.ID, updateMappingReq)
    if err != nil {
        h.Logger.Errorw("failed to update mapping metadata",
            "error", err,
            "mapping_id", mapping.ID)
        // Not critical, just log
    }

    h.Logger.Infow("customer updated from Stripe webhook",
        "stripe_customer_id", customer.ID,
        "flexprice_customer_id", mapping.EntityID)

    return nil
}
```

### 3. Customer Service Integration

```go
package service

// CustomerServiceWithMapping provides enhanced customer operations
type CustomerServiceWithMapping struct {
    *CustomerService
    EntityMappingService EntityIntegrationMappingService
    StripeService       StripeService
}

// GetCustomerWithProviderInfo returns customer with provider information
func (s *CustomerServiceWithMapping) GetCustomerWithProviderInfo(
    ctx context.Context,
    customerID string,
) (*dto.CustomerWithProviderInfoResponse, error) {
    // 1. Get customer
    customer, err := s.GetCustomer(ctx, customerID)
    if err != nil {
        return nil, err
    }

    // 2. Get all provider mappings
    filter := &types.EntityIntegrationMappingFilter{
        EntityID:   customerID,
        EntityType: "customer",
    }
    
    mappings, err := s.EntityMappingService.GetEntityIntegrationMappings(ctx, filter)
    if err != nil {
        s.Logger.Errorw("failed to get customer mappings",
            "error", err,
            "customer_id", customerID)
        // Don't fail, just return customer without provider info
    }

    // 3. Build provider info
    providerInfo := make(map[string]dto.ProviderInfo)
    for _, mapping := range mappings.EntityIntegrationMappings {
        providerInfo[mapping.ProviderType] = dto.ProviderInfo{
            ProviderEntityID: mapping.ProviderEntityID,
            Metadata:         mapping.Metadata,
            CreatedAt:        mapping.CreatedAt,
            UpdatedAt:        mapping.UpdatedAt,
        }
    }

    return &dto.CustomerWithProviderInfoResponse{
        Customer:     customer,
        ProviderInfo: providerInfo,
    }, nil
}

// DeleteCustomerWithProviderCleanup deletes customer and cleans up provider mappings
func (s *CustomerServiceWithMapping) DeleteCustomerWithProviderCleanup(
    ctx context.Context,
    customerID string,
) error {
    // 1. Get all mappings for this customer
    filter := &types.EntityIntegrationMappingFilter{
        EntityID:   customerID,
        EntityType: "customer",
    }
    
    mappings, err := s.EntityMappingService.GetEntityIntegrationMappings(ctx, filter)
    if err != nil {
        s.Logger.Errorw("failed to get customer mappings for cleanup",
            "error", err,
            "customer_id", customerID)
        // Continue with deletion even if we can't get mappings
    }

    // 2. Delete customer on providers (optional)
    for _, mapping := range mappings.EntityIntegrationMappings {
        switch mapping.ProviderType {
        case "stripe":
            err := s.StripeService.DeleteCustomer(ctx, mapping.ProviderEntityID)
            if err != nil {
                s.Logger.Errorw("failed to delete Stripe customer",
                    "error", err,
                    "stripe_customer_id", mapping.ProviderEntityID)
                // Continue with other providers
            }
        }
        
        // Delete mapping
        err := s.EntityMappingService.DeleteEntityIntegrationMapping(ctx, mapping.ID)
        if err != nil {
            s.Logger.Errorw("failed to delete mapping",
                "error", err,
                "mapping_id", mapping.ID)
        }
    }

    // 3. Delete customer on FlexPrice
    return s.DeleteCustomer(ctx, customerID)
}
```

### 4. DTO Extensions

```go
package dto

// CustomerWithProviderInfoResponse extends customer response with provider info
type CustomerWithProviderInfoResponse struct {
    *CustomerResponse
    ProviderInfo map[string]ProviderInfo `json:"provider_info"`
}

// ProviderInfo contains provider-specific information
type ProviderInfo struct {
    ProviderEntityID string                 `json:"provider_entity_id"`
    Metadata         map[string]interface{} `json:"metadata"`
    CreatedAt        string                 `json:"created_at"`
    UpdatedAt        string                 `json:"updated_at"`
}

// CreateStripeCustomerRequest for creating Stripe customers
type CreateStripeCustomerRequest struct {
    Email    string            `json:"email" validate:"required,email"`
    Name     string            `json:"name,omitempty"`
    Metadata map[string]string `json:"metadata,omitempty"`
}
```

## Usage Examples

### 1. Create Customer with Stripe Sync

```go
// Create customer service with mapping capabilities
customerService := &CustomerServiceWithMapping{
    CustomerService:       baseCustomerService,
    EntityMappingService:  entityMappingService,
    StripeService:        stripeService,
}

// Create customer and sync to Stripe
customer, err := customerService.CreateCustomerWithStripeSync(ctx, dto.CreateCustomerRequest{
    Name:  "John Doe",
    Email: "john@example.com",
})
```

### 2. Handle Stripe Webhook

```go
// Handle webhook
webhookHandler := &StripeWebhookHandler{
    CustomerService:      customerService,
    EntityMappingService: entityMappingService,
    Logger:              logger,
}

err := webhookHandler.HandleCustomerCreated(ctx, stripeEvent)
```

### 3. Get Customer with Provider Info

```go
// Get customer with all provider information
customerWithProviders, err := customerService.GetCustomerWithProviderInfo(ctx, "cust_123")

// Access provider info
if stripeInfo, exists := customerWithProviders.ProviderInfo["stripe"]; exists {
    fmt.Printf("Stripe Customer ID: %s\n", stripeInfo.ProviderEntityID)
}
```

## Benefits

1. **Duplicate Prevention**: Mappings prevent creating duplicate entities
2. **Bidirectional Sync**: Handle both FlexPrice → Stripe and Stripe → FlexPrice
3. **Audit Trail**: Track all synchronization activities
4. **Error Recovery**: Failed mappings can be retried
5. **Provider Agnostic**: Same pattern works for other providers
6. **Observability**: Rich logging and monitoring capabilities

## Testing

```go
func TestCustomerServiceWithMapping(t *testing.T) {
    suite := testutil.NewBaseServiceSuite(t)
    defer suite.Cleanup()

    customerService := &CustomerServiceWithMapping{
        CustomerService:       NewCustomerService(suite.ServiceParams),
        EntityMappingService:  NewEntityIntegrationMappingService(suite.ServiceParams),
        StripeService:        NewStripeService(suite.ServiceParams),
    }

    // Test customer creation with sync
    customer, err := customerService.CreateCustomerWithStripeSync(ctx, dto.CreateCustomerRequest{
        Name:  "Test Customer",
        Email: "test@example.com",
    })
    require.NoError(t, err)
    assert.NotNil(t, customer)

    // Verify mapping was created
    mapping, err := customerService.EntityMappingService.GetByEntityAndProvider(
        ctx, customer.ID, "customer", "stripe")
    require.NoError(t, err)
    assert.NotNil(t, mapping)
    assert.Equal(t, customer.ID, mapping.EntityID)
}
``` 