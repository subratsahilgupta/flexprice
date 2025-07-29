# Discount Engine

The discount engine is a rule-based system that allows applying discounts at both subscription and subscription line item levels, similar to Stripe's discount system.

## Architecture

The discount engine consists of three main entities:

### 1. Coupon
The coupon entity contains all the configuration and rules for applying discounts.

**Key Features:**
- **Fixed or Percentage Discounts**: Support for both fixed amount and percentage-based discounts
- **Time-based Validity**: Configurable start and end dates for coupon redemption
- **Usage Limits**: Maximum number of redemptions per coupon
- **Rule Engine**: JSON-based rules for conditional discount application
- **Cadence Types**: Once, repeated, or forever discounts

**Fields:**
```json
{
  "id": "string",
  "name": "string",
  "redeem_after": "timestamp",
  "redeem_before": "timestamp", 
  "max_redemptions": "integer",
  "total_redemptions": "integer",
  "rules": "jsonb",
  "amount_off": "decimal",
  "percentage_off": "decimal",
  "type": "enum['fixed', 'percentage']",
  "cadence": "enum['once', 'repeated', 'forever']",
  "is_active": "boolean",
  "currency": "string"
}
```

### 2. Discount
The discount entity associates coupons with subscriptions or subscription line items.

**Fields:**
```json
{
  "id": "string",
  "coupon_id": "string",
  "subscription_id": "string",
  "subscription_line_item_id": "string",
  "tenant_id": "string",
  "environment_id": "string"
}
```

### 3. Redemption
The redemption entity tracks when coupons are applied to invoices.

**Fields:**
```json
{
  "id": "string",
  "coupon_id": "string",
  "discount_id": "string",
  "invoice_id": "string",
  "invoice_line_item_id": "string",
  "redeemed_at": "timestamp",
  "original_price": "decimal",
  "final_price": "decimal",
  "discounted_amount": "decimal",
  "discount_type": "enum['fixed', 'percentage']",
  "discount_percentage": "decimal",
  "currency": "string",
  "coupon_snapshot": "jsonb"
}
```

## Rule Engine

The discount engine supports a flexible rule system for conditional discount application.

### Rule Structure
```json
{
  "inclusions": [
    {
      "field": "customer_id",
      "operator": "equals",
      "value": "cust_123"
    },
    {
      "field": "amount",
      "operator": "greater_than",
      "value": 100.0
    }
  ],
  "exclusions": [
    {
      "field": "plan_id",
      "operator": "equals",
      "value": "plan_free"
    }
  ]
}
```

### Supported Fields
- `customer_id`: Customer identifier
- `amount`: Invoice or line item amount
- `plan_id`: Plan identifier
- `subscription_id`: Subscription identifier

### Supported Operators
- `equals`: Exact match
- `not_equals`: Not equal
- `greater_than`: Greater than
- `greater_than_or_equal`: Greater than or equal
- `less_than`: Less than
- `less_than_or_equal`: Less than or equal
- `in`: Value in array
- `not_in`: Value not in array

## Usage Examples

### Creating a Fixed Amount Coupon
```go
coupon := &coupon.Coupon{
    Name:          "WELCOME10",
    AmountOff:     decimal.NewFromInt(1000), // $10.00
    Type:          types.DiscountTypeFixed,
    Cadence:       types.DiscountCadenceOnce,
    Currency:      "usd",
    IsActive:      true,
    MaxRedemptions: 100,
    RedeemBefore:  time.Now().AddDate(0, 1, 0), // Valid for 1 month
}
```

### Creating a Percentage Coupon with Rules
```go
coupon := &coupon.Coupon{
    Name:         "LOYALTY20",
    PercentageOff: decimal.NewFromInt(20), // 20%
    Type:         types.DiscountTypePercentage,
    Cadence:      types.DiscountCadenceForever,
    Currency:     "usd",
    IsActive:     true,
    Rules: map[string]interface{}{
        "conditions": []map[string]interface{}{
            {
                "field":    "amount",
                "operator": "greater_than",
                "value":    50.0,
            },
        },
    },
}
```

### Applying Discounts to an Invoice
```go
// Create discount engine service
discountEngine := NewDiscountEngineService(couponRepo, discountRepo, redemptionRepo)

// Apply discounts to invoice
applications, err := discountEngine.ApplyDiscountToInvoice(
    ctx,
    invoiceID,
    customerID,
    subscriptionID,
    lineItems,
)

// Create redemption records for applied discounts
for _, app := range applications {
    if app.Applied {
        redemption, err := discountEngine.CreateRedemption(
            ctx,
            app,
            invoiceID,
            nil, // nil for invoice-level redemption
        )
    }
}
```

## API Endpoints

### Coupons
- `POST /api/v1/coupons` - Create a new coupon
- `GET /api/v1/coupons` - List coupons
- `GET /api/v1/coupons/{id}` - Get coupon by ID
- `PUT /api/v1/coupons/{id}` - Update coupon
- `DELETE /api/v1/coupons/{id}` - Delete coupon

### Discounts
- `POST /api/v1/discounts` - Create a new discount
- `GET /api/v1/discounts` - List discounts
- `GET /api/v1/discounts/{id}` - Get discount by ID
- `DELETE /api/v1/discounts/{id}` - Delete discount

### Redemptions
- `GET /api/v1/redemptions` - List redemptions
- `GET /api/v1/redemptions/{id}` - Get redemption by ID

## Integration with Billing

The discount engine integrates with the billing system in the following ways:

1. **Invoice Generation**: Discounts are applied during invoice generation
2. **Line Item Processing**: Line item discounts are applied individually
3. **Redemption Tracking**: All discount applications are tracked for audit purposes
4. **Coupon Snapshot**: Coupon configuration is frozen at redemption time

## Best Practices

1. **Rule Design**: Keep rules simple and testable
2. **Coupon Naming**: Use descriptive names for easy identification
3. **Usage Limits**: Set appropriate max redemptions to prevent abuse
4. **Time Limits**: Use redeem_before to prevent expired coupon usage
5. **Testing**: Test discount rules thoroughly before production use

## Future Enhancements

1. **Advanced Rule Engine**: Support for complex nested conditions
2. **Bulk Operations**: Support for bulk coupon creation and application
3. **Analytics**: Discount usage analytics and reporting
4. **Webhook Integration**: Notifications for discount applications
5. **A/B Testing**: Support for discount experimentation