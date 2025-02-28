# Environment Mixin Implementation

This document summarizes the implementation of the environment mixin, which adds an `environment_id` field to entities.

## What's Done

1. Created an environment mixin in `ent/schema/mixin/environment.go` that adds an `environment_id` field to entities.
2. Added the environment mixin to all entities except auth, user, tenant, and environment.
3. Updated the query interface to include an `ApplyEnvironmentFilter` method.
4. Updated the query builder to apply the environment filter.
5. Updated domain models to include the `environment_id` field:
   - Customer
   - Feature
   - Subscription
   - Invoice
   - Entitlement
   - Meter
   - Plan
   - Price
   - Payment
   - PaymentAttempt
   - Wallet
   - WalletTransaction
   - Task
6. Updated repository implementations to handle the `environment_id` field:
   - CustomerRepository
   - FeatureRepository
   - SubscriptionRepository
   - InvoiceRepository
   - EntitlementRepository
   - MeterRepository
   - PlanRepository
   - PriceRepository
   - PaymentRepository
   - WalletRepository
   - TaskRepository
7. Updated composite indexes to include `environment_id` for better query performance.
8. Created scripts to help with the implementation.

## Next Steps

1. Update all domain models to include the `environment_id` field.
2. Update all repository implementations to handle the `environment_id` field.
3. Update all response DTOs to include the `environment_id` field.

## Implementation Details

### Environment Mixin

The environment mixin adds an `environment_id` field to entities:

```go
// EnvironmentMixin implements the ent.Mixin for sharing environment_id field with package schemas.
type EnvironmentMixin struct {
	mixin.Schema
}

// Fields of the EnvironmentMixin.
func (EnvironmentMixin) Fields() []ent.Field {
	return []ent.Field{
		field.String("environment_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Optional().
			Default(""),
	}
}
```

### Query Interface

The query interface has been updated to include an `ApplyEnvironmentFilter` method:

```go
// BaseQueryOptions defines common operations for query builders
type BaseQueryOptions[T any] interface {
	// ... existing methods ...
	// ApplyEnvironmentFilter applies environment isolation if environment ID is present in context
	ApplyEnvironmentFilter(ctx context.Context, query T) T
}
```

### Query Builder

The query builder has been updated to apply the environment filter:

```go
// ApplyBaseFilters applies common filters like tenant ID and status using the query options
func ApplyBaseFilters[T any](ctx context.Context, query T, filter types.BaseFilter, opts BaseQueryOptions[T]) T {
	query = opts.ApplyTenantFilter(ctx, query)
	query = opts.ApplyStatusFilter(query, filter.GetStatus())
	query = opts.ApplyEnvironmentFilter(ctx, query)
	return query
}
```

### Domain Models

Domain models have been updated to include the `environment_id` field:

```go
type Feature struct {
	// ... existing fields ...
	EnvironmentID string `json:"environment_id"`
	types.BaseModel
}
```

### Repository Implementations

Repository implementations have been updated to handle the `environment_id` field:

```go
func (o FeatureQueryOptions) ApplyEnvironmentFilter(ctx context.Context, query FeatureQuery) FeatureQuery {
	environmentID := types.GetEnvironmentID(ctx)
	if environmentID != "" {
		return query.Where(feature.EnvironmentIDEQ(environmentID))
	}
	return query
}
```

And the Create method has been updated to set the `environment_id` from context if not already set:

```go
// Set environment ID from context if not already set
if f.EnvironmentID == "" {
	f.EnvironmentID = types.GetEnvironmentID(ctx)
}
```

### Database Indexes

Composite indexes have been updated to include `environment_id` for better query performance:

```go
// Before
index.Fields("tenant_id", "lookup_key")

// After
index.Fields("tenant_id", "environment_id", "lookup_key")
```

This ensures that queries filtering by both tenant and environment can use efficient indexes.

## Scripts

1. `scripts/add_environment_mixin.sh`: Adds the environment mixin to all entities except auth, user, tenant, and environment.
2. `scripts/add_environment_filter.sh`: Adds the `ApplyEnvironmentFilter` method to all repository implementations.
3. `scripts/regenerate_ent.sh`: Regenerates the ent code to include the `environment_id` field.
4. `scripts/update_environment_indexes.sh`: Updates composite indexes to include `environment_id`.

## Manual Steps

After running the scripts, you'll need to:

1. Update all domain models to include the `environment_id` field.
2. Update all repository implementations to handle the `environment_id` field.
3. Update all response DTOs to include the `environment_id` field.

## Testing

To test the implementation:

1. Set the `X-Environment-ID` header in requests.
2. Verify that the `environment_id` field is set in the database.
3. Verify that queries filter by `environment_id` when the header is set. 