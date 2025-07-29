package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
)

// Discount holds the schema definition for the Discount entity.
type Discount struct {
	ent.Schema
}

// Mixin of the Discount.
func (Discount) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
		baseMixin.EnvironmentMixin{},
	}
}

// Fields of the Discount.
func (Discount) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Unique().
			Immutable(),
		field.String("coupon_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Immutable(),
		field.String("subscription_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Optional().
			Nillable(),
		field.String("subscription_line_item_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Optional().
			Nillable(),
	}
}

// Edges of the Discount.
func (Discount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("coupon", Coupon.Type).
			Ref("discounts").
			Field("coupon_id").
			Unique().
			Required(),
		edge.From("subscription", Subscription.Type).
			Ref("discounts").
			Field("subscription_id").
			Unique(),
		edge.From("subscription_line_item", SubscriptionLineItem.Type).
			Ref("discounts").
			Field("subscription_line_item_id").
			Unique(),
		edge.To("redemptions", Redemption.Type).
			Comment("Discount can have multiple redemptions"),
	}
}

// Indexes of the Discount.
func (Discount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "environment_id"),
		index.Fields("tenant_id", "environment_id", "coupon_id"),
		index.Fields("tenant_id", "environment_id", "subscription_id"),
	}
}
