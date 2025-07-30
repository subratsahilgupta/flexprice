# Entity Integration Mapping - Quick Reference

## 🚀 Quick Start

### 1. Create a Mapping

```bash
curl -X POST 'https://api.flexprice.com/v1/entity-integration-mappings' \
  -H 'x-api-key: YOUR_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{
    "entity_id": "cust_123",
    "entity_type": "customer",
    "provider_type": "stripe",
    "provider_entity_id": "cus_stripe_456",
    "metadata": {
      "stripe_customer_email": "user@example.com",
      "sync_direction": "flexprice_to_provider"
    }
  }'
```

### 2. Find Existing Mapping

```bash
# By FlexPrice entity and provider
curl -X GET 'https://api.flexprice.com/v1/entity-integration-mappings/by-entity-and-provider?entity_id=cust_123&entity_type=customer&provider_type=stripe' \
  -H 'x-api-key: YOUR_API_KEY'

# By provider entity ID
curl -X GET 'https://api.flexprice.com/v1/entity-integration-mappings/by-provider-entity?provider_type=stripe&provider_entity_id=cus_stripe_456' \
  -H 'x-api-key: YOUR_API_KEY'
```

### 3. List Mappings

```bash
# All mappings for a customer
curl -X GET 'https://api.flexprice.com/v1/entity-integration-mappings?entity_id=cust_123&entity_type=customer' \
  -H 'x-api-key: YOUR_API_KEY'

# All Stripe mappings
curl -X GET 'https://api.flexprice.com/v1/entity-integration-mappings?provider_type=stripe' \
  -H 'x-api-key: YOUR_API_KEY'
```

## 📋 Entity Types

| Type | Description | Example FlexPrice ID | Example Provider ID |
|------|-------------|---------------------|-------------------|
| `customer` | Customer entities | `cust_123` | `cus_stripe_456` |
| `plan` | Subscription plans | `plan_123` | `price_stripe_456` |
| `invoice` | Billing invoices | `inv_123` | `in_stripe_456` |
| `subscription` | Customer subscriptions | `sub_123` | `sub_stripe_456` |
| `payment` | Payment transactions | `pay_123` | `pi_stripe_456` |

## 🔌 Provider Types

| Provider | Type | Documentation |
|----------|------|---------------|
| Stripe | `stripe` | [Stripe API](https://stripe.com/docs/api) |
| Razorpay | `razorpay` | [Razorpay API](https://razorpay.com/docs/api/) |
| PayPal | `paypal` | [PayPal API](https://developer.paypal.com/docs/api/) |

## 💡 Common Use Cases

### Webhook Handler Pattern

```go
// Handle Stripe customer.created webhook
func handleStripeCustomerCreated(ctx context.Context, stripeCustomer *stripe.Customer) error {
    // Check if mapping exists
    existingMapping, err := entityMappingService.GetByProviderEntity(
        ctx, "stripe", stripeCustomer.ID)
    
    if err != nil {
        if errors.Is(err, types.ErrNotFound) {
            // Create new FlexPrice customer
            customer, err := customerService.CreateCustomer(ctx, customerReq)
            if err != nil {
                return err
            }
            
            // Create mapping
            mappingReq := dto.CreateEntityIntegrationMappingRequest{
                EntityID:         customer.ID,
                EntityType:       "customer",
                ProviderType:     "stripe",
                ProviderEntityID: stripeCustomer.ID,
                Metadata: map[string]interface{}{
                    "webhook_event": "customer.created",
                    "sync_direction": "provider_to_flexprice",
                },
            }
            _, err = entityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
            return err
        }
        return err
    }
    
    // Mapping exists, update if needed
    return updateExistingMapping(ctx, existingMapping, stripeCustomer)
}
```

### Outbound Sync Pattern

```go
// Sync FlexPrice customer to Stripe
func syncCustomerToStripe(ctx context.Context, customerID string) error {
    // Check if mapping exists
    existingMapping, err := entityMappingService.GetByEntityAndProvider(
        ctx, customerID, "customer", "stripe")
    
    if existingMapping != nil {
        // Mapping exists, return existing provider ID
        return existingMapping.ProviderEntityID, nil
    }
    
    // Create customer on Stripe
    stripeCustomer, err := stripeService.CreateCustomer(ctx, stripeReq)
    if err != nil {
        return err
    }
    
    // Create mapping
    mappingReq := dto.CreateEntityIntegrationMappingRequest{
        EntityID:         customerID,
        EntityType:       "customer",
        ProviderType:     "stripe",
        ProviderEntityID: stripeCustomer.ID,
        Metadata: map[string]interface{}{
            "sync_direction": "flexprice_to_provider",
        },
    }
    _, err = entityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
    return err
}
```

## 🔍 Query Examples

### Find by FlexPrice Entity

```go
// Get all mappings for a customer
mappings, err := service.ListByEntity(ctx, "cust_123", "customer")
```

### Find by Provider Entity

```go
// Get mapping by Stripe customer ID
mapping, err := service.GetByProviderEntity(ctx, "stripe", "cus_stripe_456")
```

### Find by Entity and Provider

```go
// Get mapping for customer + Stripe combination
mapping, err := service.GetByEntityAndProvider(ctx, "cust_123", "customer", "stripe")
```

## 📊 Metadata Examples

### FlexPrice → Provider Sync

```json
{
  "sync_direction": "flexprice_to_provider",
  "provider_customer_email": "user@example.com",
  "provider_customer_name": "John Doe",
  "created_via": "api"
}
```

### Provider → FlexPrice Sync

```json
{
  "sync_direction": "provider_to_flexprice",
  "webhook_event": "customer.created",
  "webhook_id": "evt_stripe_123",
  "provider_metadata": {
    "stripe_customer_email": "user@example.com"
  }
}
```

### Update Operations

```json
{
  "sync_direction": "bidirectional",
  "last_sync_at": "2024-01-15T10:30:00Z",
  "sync_status": "success",
  "provider_customer_email": "updated@example.com"
}
```

## ⚠️ Error Handling

### Common Error Codes

| Code | Description | Solution |
|------|-------------|----------|
| `400` | Bad Request | Check request body and parameters |
| `401` | Unauthorized | Verify API key |
| `404` | Not Found | Mapping doesn't exist |
| `409` | Conflict | Mapping already exists |
| `500` | Internal Error | Contact support |

### Error Response Format

```json
{
  "success": false,
  "error": {
    "message": "Entity integration mapping not found",
    "details": {
      "entity_id": "cust_123",
      "provider_type": "stripe"
    }
  }
}
```

## 🛠️ Development

### Local Testing

```bash
# Start local server
make run

# Test endpoints
curl -X POST 'http://localhost:8080/v1/entity-integration-mappings' \
  -H 'x-api-key: test-key' \
  -H 'Content-Type: application/json' \
  -d '{
    "entity_id": "cust_test_123",
    "entity_type": "customer",
    "provider_type": "stripe",
    "provider_entity_id": "cus_test_456"
  }'
```

### Run Tests

```bash
# Run all tests
make test

# Run specific test
go test -v ./internal/service -run TestEntityIntegrationMappingService
```

## 📚 Additional Resources

- [Full Documentation](./entity-integration-mapping.md)
- [API Specification](./swagger/entity-integration-mapping.yaml)
- [Integration Examples](./examples/)
- [Troubleshooting Guide](./troubleshooting.md) 