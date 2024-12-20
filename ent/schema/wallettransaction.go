package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// WalletTransaction holds the schema definition for the WalletTransaction entity.
type WalletTransaction struct {
	ent.Schema
}

// Fields of the WalletTransaction.
func (WalletTransaction) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("tenant_id").
			NotEmpty().
			Immutable(),
		field.String("wallet_id").
			NotEmpty().
			Immutable(),
		field.String("type").
			Default(string(types.TransactionTypeCredit)).
			NotEmpty(),
		field.Other("amount", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,9)",
			}),
		field.Other("balance_before", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,9)",
			}).
			Default(decimal.Zero),
		field.Other("balance_after", decimal.Decimal{}).
			SchemaType(map[string]string{
				"postgres": "numeric(20,9)",
			}).
			Default(decimal.Zero),
		field.String("reference_type").
			Optional(),
		field.String("reference_id").
			Optional(),
		field.String("description").
			Optional(),
		field.JSON("metadata", map[string]string{}).
			Optional().
			SchemaType(map[string]string{
				"postgres": "jsonb",
			}),
		field.String("transaction_status").
			Default(string(types.TransactionStatusPending)),
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

// Edges of the WalletTransaction.
func (WalletTransaction) Edges() []ent.Edge {
	return nil
}

// Indexes of the WalletTransaction.
func (WalletTransaction) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "wallet_id", "status"),
		index.Fields("tenant_id", "reference_type", "reference_id", "status"),
		index.Fields("created_at"),
	}
}
