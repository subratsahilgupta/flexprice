package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
)

var Idx_tenant_environment_external_id_unique = "idx_tenant_environment_external_id_unique"

// Customer holds the schema definition for the Customer entity.
type Customer struct {
	ent.Schema
}

// Mixin of the Customer.
func (Customer) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
		baseMixin.EnvironmentMixin{},
	}
}

// Fields of the Customer.
func (Customer) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Unique().
			Immutable(),
		field.String("external_id").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			NotEmpty(),
		field.String("name").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			NotEmpty(),
		field.String("email").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			Optional(),
		// Address fields
		field.String("address_line1").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			Optional(),
		field.String("address_line2").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			Optional(),
		field.String("address_city").
			SchemaType(map[string]string{
				"postgres": "varchar(100)",
			}).
			Optional(),
		field.String("address_state").
			SchemaType(map[string]string{
				"postgres": "varchar(100)",
			}).
			Optional(),
		field.String("address_postal_code").
			SchemaType(map[string]string{
				"postgres": "varchar(20)",
			}).
			Optional(),
		field.String("address_country").
			SchemaType(map[string]string{
				"postgres": "varchar(2)",
			}).
			Optional(),
		// Metadata as JSON field
		field.JSON("metadata", map[string]string{}).
			Optional(),
		field.Bool("auto_cancel_on_unpaid").
			Default(false), // Whether to automatically cancel subscription if invoice is unpaid after grace period
	}
}

// Edges of the Customer.
func (Customer) Edges() []ent.Edge {
	return nil
}

// Indexes of the Customer.
func (Customer) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "environment_id", "external_id").
			Unique().
			Annotations(entsql.IndexWhere("(external_id IS NOT NULL AND external_id != '') AND status = 'published'")).
			StorageKey(Idx_tenant_environment_external_id_unique),
		index.Fields("tenant_id", "environment_id"),
		index.Fields("auto_cancel_on_unpaid", "status").
			StorageKey("idx_auto_cancel_status").
			Annotations(entsql.IndexWhere("status = 'published'")),
	}
}
