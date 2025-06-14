package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type CreditGrantApplication struct {
	ent.Schema
}

func (CreditGrantApplication) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
		baseMixin.EnvironmentMixin{},
	}
}

// Fields of the CreditGrantApplication.
func (CreditGrantApplication) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Unique().
			Immutable(),

		field.String("credit_grant_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Immutable(),

		field.String("subscription_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Immutable(),

		// Timing
		field.Time("scheduled_for"),

		field.Time("applied_at").
			Optional().
			Nillable(),

		// Billing period context
		field.Time("billing_period_start"),

		field.Time("billing_period_end"),

		// Application details
		field.String("application_status").
			Default(string(types.ApplicationStatusScheduled)),

		field.Other("amount_applied", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,8)",
			}).
			Default(decimal.Zero),

		field.String("currency"),

		// Context
		field.String("application_reason"),
		field.String("subscription_status_at_application"),

		// Prorating
		field.Bool("is_prorated"),

		field.Other("proration_factor", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,8)",
			}).
			Optional(),

		// Prorating
		field.Other("full_period_amount", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,8)",
			}).
			Optional(),

		// Retry handling
		field.Int("retry_count").
			Default(0),

		field.String("failure_reason").
			Optional().
			Nillable(),

		field.Time("next_retry_at").
			Optional().
			Nillable(),

		// Metadata
		field.Other("metadata", types.Metadata{}).
			SchemaType(map[string]string{
				"postgres": "jsonb",
			}).
			Optional(),
	}
}

// Edges of the CreditGrantApplication.
func (CreditGrantApplication) Edges() []ent.Edge {
	return nil // define if you want relationships with CreditGrant, Subscription, etc.
}

func (CreditGrantApplication) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Skip(), // Tell Ent to skip managing this table
	}
}
