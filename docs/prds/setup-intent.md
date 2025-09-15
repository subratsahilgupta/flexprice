# Stripe Setup Intent Integration PRD

## Overview

This PRD outlines the integration of Stripe's Setup Intents API into FlexPrice to enable customers to securely save payment methods for future use without making an immediate payment. This feature is essential for subscription-based businesses that need to collect payment method details upfront but charge customers later.

## Background

Based on [Stripe's Setup Intents API documentation](https://docs.stripe.com/payments/setup-intents), Setup Intents are designed to:

- Set up payment methods for future payments without charging immediately
- Handle Strong Customer Authentication (SCA) requirements
- Optimize payment success rates for future transactions
- Provide compliance with payment regulations

## Business Requirements

### Primary Use Cases

1. **Subscription Onboarding**: Collect payment method during subscription setup without immediate charge
2. **Trial Periods**: Save payment method during free trial for automatic billing when trial ends
3. **Usage-Based Billing**: Store payment method for customers who will be charged based on actual usage
4. **Compliance**: Meet regulatory requirements for saving payment methods with proper customer consent

### Key Benefits

- **Improved User Experience**: Streamlined onboarding without immediate payment
- **Higher Conversion Rates**: Reduced friction during signup process
- **Regulatory Compliance**: Proper handling of SCA and mandate requirements
- **Future Payment Success**: Optimized payment methods for off-session transactions

## Technical Requirements

### API Endpoint Specification

#### Create Setup Intent Session
```
POST /v1/setup-intents/sessions
```

**Request Body:**
```json
{
  "customer_id": "string (required)",
  "usage": "on_session | off_session (optional, defaults to off_session)",
  "payment_method_types": ["card"] (optional, defaults to ["card"]),
  "success_url": "string (optional)",
  "cancel_url": "string (optional)",
  "metadata": {
    "key": "value"
  } (optional)
}
```

**Response:**
```json
{
  "setup_intent_id": "string",
  "checkout_session_id": "string",
  "checkout_url": "string",
  "client_secret": "string",
  "status": "requires_payment_method",
  "usage": "off_session",
  "customer_id": "string",
  "created_at": "timestamp",
  "expires_at": "timestamp"
}
```

### Integration Architecture

#### 1. API Layer (`internal/api/v1/setup_intent.go`)
- New handler for Setup Intent operations
- Route registration in router.go
- Request validation and error handling

#### 2. Service Layer (`internal/service/stripe.go`)
- Extend existing StripeService with Setup Intent methods
- Integration with Stripe Checkout Sessions in setup mode
- Customer validation and Stripe customer creation if needed

#### 3. DTO Layer (`internal/api/dto/setup_intent.go`)
- Request/response DTOs for Setup Intent operations
- Validation logic for Setup Intent parameters

#### 4. Webhook Handling (`internal/api/v1/webhook.go`)
- Handle `setup_intent.succeeded` events
- Handle `setup_intent.setup_failed` events
- Handle `checkout.session.completed` for setup mode sessions

### Data Flow

1. **Client Request**: Frontend calls `/v1/setup-intents/sessions` with customer details
2. **Customer Validation**: Verify customer exists in FlexPrice system
3. **Stripe Customer**: Ensure customer exists in Stripe (create if needed)
4. **Setup Intent Creation**: Create Stripe Setup Intent with specified usage
5. **Checkout Session**: Create Stripe Checkout Session in setup mode
6. **URL Response**: Return checkout URL to client
7. **Customer Interaction**: Customer completes payment method setup on Stripe-hosted page
8. **Webhook Processing**: Handle completion/failure webhooks
9. **Payment Method Storage**: Save payment method reference in customer metadata

### Usage Parameter Strategy

Following Stripe's recommendations:

- **`off_session` (default)**: For customers who will be charged when not actively using the app
  - Subscription renewals
  - Usage-based billing
  - Automatic payments
  
- **`on_session`**: For customers who will be charged during active sessions
  - Manual payments during checkout
  - One-time purchases with saved cards

### Security Considerations

1. **Customer Validation**: Ensure only valid customers can create Setup Intents
2. **Webhook Verification**: Verify webhook signatures to prevent tampering
3. **Metadata Sanitization**: Sanitize and validate all metadata inputs
4. **Environment Isolation**: Ensure proper environment-specific Stripe configurations

### Error Handling

#### Client Errors (4xx)
- Invalid customer ID
- Missing required fields
- Invalid usage parameter
- Invalid payment method types

#### Server Errors (5xx)
- Stripe API failures
- Database connection issues
- Configuration errors

### Compliance Requirements

#### Customer Consent
- Implement proper consent collection for saving payment methods
- Support for different consent types (on-session vs off-session)
- Clear communication about how payment methods will be used

#### Mandate Text
For off-session usage, include appropriate mandate text:
```
"By providing your payment information and confirming this agreement, you authorize [Company Name] to charge your payment method for future payments in accordance with their terms."
```

## Implementation Plan

### Phase 1: Core Setup Intent API
1. Create Setup Intent DTOs
2. Extend Stripe service with Setup Intent methods
3. Implement API endpoint for creating Setup Intent sessions
4. Add route registration

### Phase 2: Webhook Integration
1. Add Setup Intent webhook event types
2. Implement webhook handlers for Setup Intent events
3. Update payment method storage logic

### Phase 3: Enhanced Features
1. Support for multiple payment method types
2. Payment method management endpoints
3. Setup Intent status checking endpoints

## Testing Strategy

### Unit Tests
- DTO validation logic
- Service layer Setup Intent creation
- Webhook event processing

### Integration Tests
- End-to-end Setup Intent flow
- Webhook processing with real Stripe events
- Error handling scenarios

### Manual Testing
- Complete Setup Intent flow in Stripe test mode
- Webhook delivery verification
- Payment method storage validation

## Monitoring and Observability

### Metrics
- Setup Intent creation success/failure rates
- Webhook processing latency
- Payment method setup completion rates

### Logging
- Setup Intent creation events
- Webhook processing events
- Error conditions and failures

### Alerts
- High Setup Intent failure rates
- Webhook processing failures
- Stripe API errors

## Future Enhancements

1. **Payment Method Management**: APIs to list, update, and delete saved payment methods
2. **Multi-Gateway Support**: Extend Setup Intent concept to other payment gateways
3. **Advanced Validation**: Enhanced fraud detection during setup
4. **Customer Portal**: Self-service payment method management for customers

## Success Metrics

- **Setup Intent Success Rate**: >95% successful setup completions
- **API Response Time**: <500ms for Setup Intent creation
- **Webhook Processing**: <2s processing time for Setup Intent webhooks
- **Customer Adoption**: Track usage of saved payment methods for future payments

## Dependencies

- Existing Stripe integration (`internal/service/stripe.go`)
- Customer management system
- Webhook infrastructure
- Environment configuration management

## Risks and Mitigation

### Technical Risks
- **Stripe API Changes**: Monitor Stripe API updates and maintain compatibility
- **Webhook Reliability**: Implement retry logic and dead letter queues

### Business Risks
- **Compliance Issues**: Regular review of mandate text and consent flows
- **Customer Experience**: Monitor setup completion rates and optimize flow

## Conclusion

The Setup Intent integration will provide FlexPrice customers with a seamless way to save payment methods for future use, improving the overall user experience while maintaining compliance with payment regulations. The implementation follows Stripe's best practices and integrates cleanly with the existing FlexPrice architecture.
