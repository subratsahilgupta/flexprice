package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// RevenueFact is a derived, day-grain slice of billed/recognized revenue for a
// subscription line item. It is written by the revenue rollup (non-mutating
// billing re-run) and reconciles to invoices by construction. Non-partitioned
// in this slice; no BaseMixin — the audit columns are computed_at + version,
// not created_at/updated_at/created_by/updated_by.
type RevenueFact struct {
	ent.Schema
}

func (RevenueFact) Fields() []ent.Field {
	numeric := map[string]string{"postgres": "numeric(38,9)"}
	text := map[string]string{"postgres": "text"}
	date := map[string]string{"postgres": "date"}

	return []ent.Field{
		field.String("id").SchemaType(text).Unique().Immutable(),

		field.String("tenant_id").SchemaType(text),
		field.String("environment_id").SchemaType(text),
		field.String("customer_id").SchemaType(text),
		field.String("subscription_id").SchemaType(text),
		field.String("sub_line_item_id").SchemaType(text).Optional().Nillable(),
		field.String("price_id").SchemaType(text).Optional().Nillable(),
		field.String("meter_id").SchemaType(text).Optional().Nillable(),

		field.String("aggregation_type").GoType(types.AggregationType("")).SchemaType(text).Optional().Nillable(),
		field.String("revenue_source").GoType(types.RevenueSource("")).SchemaType(text),

		field.Time("period_start").SchemaType(date),
		field.Time("period_end").SchemaType(date),
		field.Time("day").SchemaType(date),

		// recognition-ready (Phase 4), unused in this slice
		field.Time("service_start").SchemaType(date).Optional().Nillable(),
		field.Time("service_end").SchemaType(date).Optional().Nillable(),
		field.String("recognition_method").GoType(types.RecognitionMethod("")).SchemaType(text).Optional().Nillable(),

		field.Other("usage_at_list_rate", decimal.Decimal{}).SchemaType(numeric).Default(decimal.Zero),
		field.Other("tier_delta", decimal.Decimal{}).SchemaType(numeric).Default(decimal.Zero),
		field.Other("entitlement_amount", decimal.Decimal{}).SchemaType(numeric).Default(decimal.Zero),
		field.Other("line_discount", decimal.Decimal{}).SchemaType(numeric).Default(decimal.Zero),
		field.Other("invoice_discount", decimal.Decimal{}).SchemaType(numeric).Default(decimal.Zero),
		field.Other("net_amount", decimal.Decimal{}).SchemaType(numeric),
		field.Other("billable_qty", decimal.Decimal{}).SchemaType(numeric).Default(decimal.Zero),
		field.Other("entitlement_qty", decimal.Decimal{}).SchemaType(numeric).Default(decimal.Zero),

		field.String("decomposition_mode").GoType(types.DecompositionMode("")).SchemaType(text),
		field.String("currency").SchemaType(text),
		field.String("status").GoType(types.FactStatus("")).SchemaType(text),
		field.Bool("is_revert").Default(false),

		field.String("invoice_id").SchemaType(text).Optional().Nillable(),
		field.String("invoice_line_item_id").SchemaType(text).Optional().Nillable(),

		// recognition day given accounting-period lock: equals `day` while the
		// period is open, shifts to the next open period's first day once the
		// real period is closed (a catch-up so a closed month is never rewritten).
		// Reserved for Phase-4 recognition; unused in this slice.
		field.Time("lock_adjusted_day").SchemaType(date).Optional().Nillable(),

		field.Time("computed_at").
			SchemaType(map[string]string{"postgres": "timestamptz"}).
			Default(time.Now).
			Annotations(entsql.Annotation{Default: "now()"}),
		field.Int64("version").Default(1),
	}
}

func (RevenueFact) Indexes() []ent.Index {
	return []ent.Index{
		// exactly one LIVE provisional row per grain — drives the ON CONFLICT upsert
		index.Fields("tenant_id", "environment_id", "subscription_id", "price_id", "day", "revenue_source").
			Unique().
			Annotations(entsql.IndexWhere("status = 'PROVISIONAL'")).
			StorageKey("revenue_facts_provisional_grain"),
		index.Fields("tenant_id", "environment_id", "day", "revenue_source").
			StorageKey("revenue_facts_read"),
		index.Fields("tenant_id", "environment_id", "invoice_id").
			StorageKey("revenue_facts_invoice"),
	}
}

func (RevenueFact) Edges() []ent.Edge { return nil }
