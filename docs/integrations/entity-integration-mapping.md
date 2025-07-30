# Entity Integration Mapping

## Overview

The Entity Integration Mapping system provides a generic framework for tracking bidirectional synchronization between FlexPrice entities and external payment providers. This system enables seamless integration with multiple payment providers (Stripe, Razorpay, etc.) while maintaining data consistency and traceability.

## Architecture

### Core Components

- **Entity Integration Mapping**: Central table storing relationships between FlexPrice entities and provider entities
- **Service Layer**: Business logic for CRUD operations and specialized queries
- **Repository Layer**: Data access layer with ENT implementation
- **DTO Layer**: Request/response data transfer objects

### Database Schema

```sql
CREATE TABLE entity_integration_mappings (
    id VARCHAR(255) PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    environment_id VARCHAR(255) NOT NULL,
    entity_id VARCHAR(255) NOT NULL,           -- FlexPrice entity ID
    entity_type VARCHAR(50) NOT NULL,          -- customer, plan, invoice, etc.
    provider_type VARCHAR(50) NOT NULL,        -- stripe, razorpay, etc.
    provider_entity_id VARCHAR(255) NOT NULL,  -- Provider's entity ID
    metadata JSONB,                           -- Additional provider-specific data
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    status VARCHAR(20) DEFAULT 'active'
);
```

## Use Cases

### 1. FlexPrice → Provider Synchronization

When creating entities on FlexPrice and syncing to external providers:

```go
// Create customer on FlexPrice
customer, err := customerService.CreateCustomer(ctx, customerReq)

// Sync to Stripe
stripeCustomer, err := stripeService.CreateCustomer(ctx, stripeReq)

// Create mapping
mappingReq := dto.CreateEntityIntegrationMappingRequest{
    EntityID:         customer.ID,
    EntityType:       "customer",
    ProviderType:     "stripe",
    ProviderEntityID: stripeCustomer.ID,
    Metadata: map[string]interface{}{
        "stripe_customer_email": customer.Email,
        "sync_direction":        "flexprice_to_provider",
    },
}
mapping, err := entityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
```

### 2. Provider → FlexPrice Synchronization

When receiving webhooks from external providers:

```go
// Receive Stripe webhook
stripeCustomer := webhookEvent.Data.Object

// Check if mapping exists
existingMapping, err := entityMappingService.GetByProviderEntity(
    ctx, "stripe", stripeCustomer.ID)

if err != nil {
    // Create new FlexPrice customer
    customer, err := customerService.CreateCustomer(ctx, customerReq)
    
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
    mapping, err := entityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
}
```

### 3. Bidirectional Sync Prevention

Prevent duplicate entities during synchronization:

```go
// Before creating customer on provider, check if mapping exists
existingMapping, err := entityMappingService.GetByEntityAndProvider(
    ctx, customerID, "customer", "stripe")

if existingMapping != nil {
    // Mapping exists, skip creation
    return existingMapping.ProviderEntityID, nil
}

// Create new mapping
// ... create customer and mapping
```

## API Endpoints

### Create Entity Integration Mapping

```http
POST /v1/entity-integration-mappings
```

**Request Body:**
```json
{
  "entity_id": "cust_123",
  "entity_type": "customer",
  "provider_type": "stripe",
  "provider_entity_id": "cus_stripe_456",
  "metadata": {
    "stripe_customer_email": "user@example.com",
    "sync_direction": "flexprice_to_provider"
  }
}
```

**Response:**
```json
{
  "success": true,
  "entity_integration_mapping": {
    "id": "eim_01K1D4YZR7XD15BW6WXD6N3EE4",
    "entity_id": "cust_123",
    "entity_type": "customer",
    "provider_type": "stripe",
    "provider_entity_id": "cus_stripe_456",
    "metadata": {
      "stripe_customer_email": "user@example.com",
      "sync_direction": "flexprice_to_provider"
    },
    "tenant_id": "tenant_123",
    "environment_id": "env_456",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
}
```

### Get Entity Integration Mapping

```http
GET /v1/entity-integration-mappings/{id}
```

### Update Entity Integration Mapping

```http
PUT /v1/entity-integration-mappings/{id}
```

**Request Body:**
```json
{
  "entity_id": "cust_123",
  "entity_type": "customer",
  "provider_type": "stripe",
  "provider_entity_id": "cus_stripe_456",
  "metadata": {
    "stripe_customer_email": "updated@example.com",
    "sync_direction": "flexprice_to_provider"
  }
}
```

### Delete Entity Integration Mapping

```http
DELETE /v1/entity-integration-mappings/{id}
```

### List Entity Integration Mappings

```http
GET /v1/entity-integration-mappings
```

**Query Parameters:**
- `entity_id`: Filter by FlexPrice entity ID
- `entity_type`: Filter by entity type (customer, plan, invoice, etc.)
- `provider_type`: Filter by provider type (stripe, razorpay, etc.)
- `provider_entity_id`: Filter by provider entity ID
- `limit`: Number of results (default: 20)
- `offset`: Pagination offset (default: 0)

### Get by Entity and Provider

```http
GET /v1/entity-integration-mappings/by-entity-and-provider
```

**Query Parameters:**
- `entity_id`: FlexPrice entity ID
- `entity_type`: Entity type
- `provider_type`: Provider type

### Get by Provider Entity

```http
GET /v1/entity-integration-mappings/by-provider-entity
```

**Query Parameters:**
- `provider_type`: Provider type
- `provider_entity_id`: Provider entity ID

## Entity Types

| Entity Type | Description | Example FlexPrice ID | Example Provider ID |
|-------------|-------------|---------------------|-------------------|
| `customer` | Customer entities | `cust_123` | `cus_stripe_456` |
| `plan` | Subscription plans | `plan_123` | `price_stripe_456` |
| `invoice` | Billing invoices | `inv_123` | `in_stripe_456` |
| `subscription` | Customer subscriptions | `sub_123` | `sub_stripe_456` |
| `payment` | Payment transactions | `pay_123` | `pi_stripe_456` |

## Provider Types

| Provider Type | Description | API Documentation |
|---------------|-------------|-------------------|
| `stripe` | Stripe payment processor | [Stripe API](https://stripe.com/docs/api) |
| `razorpay` | Razorpay payment processor | [Razorpay API](https://razorpay.com/docs/api/) |
| `paypal` | PayPal payment processor | [PayPal API](https://developer.paypal.com/docs/api/) |

## Error Handling

### Common Error Responses

**400 Bad Request:**
```json
{
  "success": false,
  "error": {
    "message": "Validation failed",
    "details": {
      "entity_id": "Entity ID is required",
      "provider_type": "Invalid provider type"
    }
  }
}
```

**404 Not Found:**
```json
{
  "success": false,
  "error": {
    "message": "Entity integration mapping not found",
    "details": {
      "id": "eim_01K1D4YZR7XD15BW6WXD6N3EE4"
    }
  }
}
```

**409 Conflict:**
```json
{
  "success": false,
  "error": {
    "message": "Entity integration mapping already exists",
    "details": {
      "entity_id": "cust_123",
      "provider_type": "stripe"
    }
  }
}
```

## Best Practices

### 1. Idempotency

Always check for existing mappings before creating new ones:

```go
// Check if mapping exists
existingMapping, err := service.GetByEntityAndProvider(
    ctx, entityID, entityType, providerType)

if existingMapping != nil {
    // Use existing mapping
    return existingMapping, nil
}

// Create new mapping
return service.CreateEntityIntegrationMapping(ctx, req)
```

### 2. Metadata Usage

Use metadata to store provider-specific information:

```go
metadata := map[string]interface{}{
    "provider_customer_email": customer.Email,
    "provider_customer_name":  customer.Name,
    "sync_direction":          "flexprice_to_provider",
    "webhook_event":           "customer.created",
    "provider_metadata":       stripeCustomer.Metadata,
}
```

### 3. Error Handling

Implement proper error handling for webhook scenarios:

```go
// Handle webhook with existing mapping
mapping, err := service.GetByProviderEntity(ctx, "stripe", stripeCustomerID)
if err != nil {
    if errors.Is(err, types.ErrNotFound) {
        // Create new FlexPrice entity and mapping
        return createNewEntityAndMapping(ctx, stripeCustomer)
    }
    return err
}

// Update existing mapping if needed
return updateExistingMapping(ctx, mapping, stripeCustomer)
```

### 4. Cleanup Strategy

Implement cleanup for deleted entities:

```go
// When deleting a FlexPrice entity, clean up mappings
mappings, err := service.ListByEntity(ctx, entityID, entityType)
for _, mapping := range mappings {
    // Optionally delete provider entity
    // Delete mapping
    service.DeleteEntityIntegrationMapping(ctx, mapping.ID)
}
```

## Monitoring and Observability

### Key Metrics

- **Sync Success Rate**: Percentage of successful entity synchronizations
- **Sync Latency**: Time taken for entity synchronization
- **Mapping Creation Rate**: Number of new mappings created per time period
- **Error Rate**: Number of synchronization errors

### Logging

```go
// Log mapping creation
logger.Infow("entity integration mapping created",
    "mapping_id", mapping.ID,
    "entity_id", mapping.EntityID,
    "entity_type", mapping.EntityType,
    "provider_type", mapping.ProviderType,
    "provider_entity_id", mapping.ProviderEntityID,
    "sync_direction", mapping.Metadata["sync_direction"])

// Log mapping lookup
logger.Debugw("entity integration mapping lookup",
    "entity_id", entityID,
    "provider_type", providerType,
    "found", mapping != nil)
```

## Security Considerations

### 1. Data Validation

- Validate all input fields before processing
- Sanitize metadata to prevent injection attacks
- Implement proper access controls

### 2. Audit Trail

- Log all mapping creation, updates, and deletions
- Track who performed each operation
- Maintain audit logs for compliance

### 3. Rate Limiting

- Implement rate limiting for API endpoints
- Prevent abuse of mapping creation endpoints
- Monitor for suspicious activity

## Future Enhancements

### 1. Advanced Filtering

- Add support for complex filtering queries
- Implement full-text search on metadata
- Add date range filtering

### 2. Bulk Operations

- Support bulk creation of mappings
- Implement batch updates
- Add bulk deletion capabilities

### 3. Webhook Integration

- Automatic mapping creation from webhooks
- Real-time synchronization status updates
- Webhook retry mechanisms

### 4. Analytics

- Sync performance analytics
- Provider usage statistics
- Error pattern analysis

## Migration Guide

### From Direct Provider Integration

If you're migrating from direct provider integration:

1. **Audit Existing Data**: Identify all existing provider entities
2. **Create Mappings**: Create mappings for existing entities
3. **Update Services**: Modify services to use mapping system
4. **Test Thoroughly**: Verify all integrations work correctly
5. **Monitor**: Watch for any issues during transition

### Example Migration Script

```go
// Migrate existing Stripe customers
customers, err := customerService.ListCustomers(ctx, filter)
for _, customer := range customers {
    if customer.ProviderCustomerID != "" {
        // Create mapping for existing customer
        mappingReq := dto.CreateEntityIntegrationMappingRequest{
            EntityID:         customer.ID,
            EntityType:       "customer",
            ProviderType:     "stripe",
            ProviderEntityID: customer.ProviderCustomerID,
            Metadata: map[string]interface{}{
                "migration_source": "legacy_integration",
                "migrated_at":      time.Now().UTC(),
            },
        }
        entityMappingService.CreateEntityIntegrationMapping(ctx, mappingReq)
    }
}
``` 