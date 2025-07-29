package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
	"github.com/shopspring/decimal"
)

// Redemption holds the schema definition for the Redemption entity.
type Redemption struct {
	ent.Schema
}

// Mixin of the Redemption.
func (Redemption) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
		baseMixin.EnvironmentMixin{},
	}
}

// Fields of the Redemption.
func (Redemption) Fields() []ent.Field {
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
		field.String("discount_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Immutable(),
		field.String("invoice_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Immutable(),
		field.String("invoice_line_item_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Optional().
			Nillable(),
		field.Time("redeemed_at").
			Default(time.Now).
			Immutable(),
		field.Other("original_price", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,8)",
			}),
		field.Other("final_price", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,8)",
			}),
		field.Other("discounted_amount", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,8)",
			}),
		field.String("discount_type").
			SchemaType(map[string]string{
				"postgres": "varchar(20)",
			}).
			NotEmpty().
			Comment("Type of discount applied: fixed or percentage"),
		field.Other("discount_percentage", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(5,4)",
			}).
			Optional().
			Nillable().
			Comment("Percentage value, only for percentage discounts"),
		field.String("currency").
			SchemaType(map[string]string{
				"postgres": "varchar(10)",
			}).
			NotEmpty(),
		field.JSON("coupon_snapshot", map[string]interface{}{}).
			Optional().
			Comment("Frozen coupon configuration at time of redemption"),
	}
}

// Edges of the Redemption.
func (Redemption) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("coupon", Coupon.Type).
			Ref("redemptions").
			Field("coupon_id").
			Unique().
			Required(),
		edge.From("discount", Discount.Type).
			Ref("redemptions").
			Field("discount_id").
			Unique().
			Required(),
		edge.From("invoice", Invoice.Type).
			Ref("redemptions").
			Field("invoice_id").
			Unique().
			Required(),
		edge.From("invoice_line_item", InvoiceLineItem.Type).
			Ref("redemptions").
			Field("invoice_line_item_id").
			Unique(),
	}
}

// Indexes of the Redemption.
func (Redemption) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "environment_id"),
		index.Fields("tenant_id", "environment_id", "coupon_id"),
		index.Fields("tenant_id", "environment_id", "invoice_id"),
	}
}
