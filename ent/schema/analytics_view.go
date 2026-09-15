package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/flexprice/flexprice/ent/schema/mixin"
	"github.com/flexprice/flexprice/internal/types"
)

// AnalyticsView holds the schema definition for the AnalyticsView entity.
type AnalyticsView struct {
	ent.Schema
}

// Mixin of the AnalyticsView.
func (AnalyticsView) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.BaseMixin{},
		mixin.EnvironmentMixin{},
	}
}

// Fields of the AnalyticsView.
func (AnalyticsView) Fields() []ent.Field {
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
		field.JSON("definition", types.ViewDefinition{}).
			SchemaType(map[string]string{
				"postgres": "jsonb",
			}),
	}
}

// Edges of the AnalyticsView.
func (AnalyticsView) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Indexes of the AnalyticsView.
func (AnalyticsView) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "environment_id", "status"),
	}
}
