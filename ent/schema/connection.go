package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
)

var Idx_tenant_environment_connection_code_unique = "idx_tenant_environment_connection_code_unique"

// Connection holds the schema definition for the Connection entity.
type Connection struct {
	ent.Schema
}

// Mixin of the Connection.
func (Connection) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
		baseMixin.EnvironmentMixin{},
	}
}

// Fields of the Connection.
func (Connection) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Unique().
			Immutable(),
		field.String("name").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			NotEmpty(),
		field.String("description").
			SchemaType(map[string]string{
				"postgres": "text",
			}).
			Optional(),
		field.String("connection_code").
			SchemaType(map[string]string{
				"postgres": "varchar(100)",
			}).
			NotEmpty(),
		field.Enum("provider_type").
			Values("flexprice", "stripe", "razorpay").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}),
		field.JSON("metadata", map[string]interface{}{}).
			Optional(),
		field.String("secret_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Optional(),
	}
}

// Edges of the Connection.
func (Connection) Edges() []ent.Edge {
	return nil
}

// Indexes of the Connection.
func (Connection) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "environment_id", "connection_code").
			Unique().
			Annotations(entsql.IndexWhere("status = 'published'")).
			StorageKey(Idx_tenant_environment_connection_code_unique),
		index.Fields("tenant_id", "environment_id"),
		index.Fields("provider_type"),
	}
}
