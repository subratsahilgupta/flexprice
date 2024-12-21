package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

// Wallet holds the schema definition for the Wallet entity.
type Wallet struct {
	ent.Schema
}

// Fields of the Wallet.
func (Wallet) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("tenant_id").
			NotEmpty().
			Immutable(),
		field.String("customer_id").
			NotEmpty().
			Immutable(),
		field.String("currency").
			NotEmpty(),
		field.String("description").
			Optional(),
		field.JSON("metadata", map[string]string{}).
			Optional().
			SchemaType(map[string]string{
				"postgres": "jsonb",
			}),
		field.Other("balance", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,9)",
			}).
			Default(decimal.Zero),
		field.String("wallet_status").
			Default("active"),
		field.String("status").
			Default("published"),
		field.Time("created_at").
			Immutable().
			Default(time.Now),
		field.String("created_by").
			Optional().
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
		field.String("updated_by").
			Optional(),
	}
}

// Edges of the Wallet.
func (Wallet) Edges() []ent.Edge {
	return nil
}

// Indexes of the Wallet.
func (Wallet) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "customer_id", "status"),
		index.Fields("tenant_id", "status", "wallet_status"),
	}
}
