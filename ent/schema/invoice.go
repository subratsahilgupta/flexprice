package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Invoice holds the schema definition for the Invoice entity.
type Invoice struct {
	ent.Schema
}

// Mixin of the Invoice.
func (Invoice) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
	}
}

// Fields of the Invoice.
func (Invoice) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("customer_id").
			NotEmpty().
			Immutable(),
		field.String("subscription_id").
			Optional().
			Nillable(),
		field.String("wallet_id").
			Optional().
			Nillable(),
		field.String("invoice_status").
			Default(string(types.InvoiceStatusDraft)),
		field.String("currency").
			NotEmpty(),
		field.Other("amount_due", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,8)",
			}).
			Default(decimal.Zero),
		field.Other("amount_paid", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,8)",
			}).
			Default(decimal.Zero),
		field.Other("amount_remaining", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,8)",
			}).
			Default(decimal.Zero),
		field.String("description").
			Optional(),
		field.Time("due_date").
			Optional().
			Nillable(),
		field.Time("paid_at").
			Optional().
			Nillable(),
		field.Time("voided_at").
			Optional().
			Nillable(),
		field.Time("finalized_at").
			Optional().
			Nillable(),
		field.String("payment_intent_id").
			Optional().
			Nillable(),
		field.String("invoice_pdf_url").
			Optional().
			Nillable(),
		field.Int("attempt_count").
			Default(0),
		field.String("billing_reason").
			Optional(),
		field.JSON("metadata", map[string]interface{}{}).
			Optional(),
		field.Int("version").
			Default(1),
	}
}

// Edges of the Invoice.
func (Invoice) Edges() []ent.Edge {
	return nil
}

// Indexes of the Invoice.
func (Invoice) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "customer_id", "status"),
		index.Fields("tenant_id", "subscription_id", "status"),
		index.Fields("tenant_id", "wallet_id", "status"),
		index.Fields("tenant_id", "status", "due_date"),
		index.Fields("tenant_id", "payment_intent_id").
			Unique(),
	}
}
