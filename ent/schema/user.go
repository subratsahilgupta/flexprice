package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Mixin of the User.
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
	}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Unique().
			Immutable(),
		field.String("email").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			NotEmpty(),
	}
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "email").
			Unique().
			StorageKey("idx_tenant_email_unique").
			Annotations(entsql.IndexWhere("status = 'published'")),
		index.Fields("tenant_id", "status").
			StorageKey("idx_tenant_status"),
		index.Fields("tenant_id", "created_at").
			StorageKey("idx_tenant_created_at"),
	}
}
