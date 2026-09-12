package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/flexprice/flexprice/ent/schema/mixin"
)

// AnalyticsSavedView holds the schema definition for the AnalyticsSavedView entity.
type AnalyticsSavedView struct {
	ent.Schema
}

// Mixin of the AnalyticsSavedView.
func (AnalyticsSavedView) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.BaseMixin{},
		mixin.EnvironmentMixin{},
	}
}

// Fields of the AnalyticsSavedView.
func (AnalyticsSavedView) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.Int("version").
			Default(1),
		field.JSON("definition", map[string]interface{}{}).
			SchemaType(map[string]string{
				"postgres": "jsonb",
			}),
	}
}

// Edges of the AnalyticsSavedView.
func (AnalyticsSavedView) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Indexes of the AnalyticsSavedView.
func (AnalyticsSavedView) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "environment_id", "status"),
	}
}
