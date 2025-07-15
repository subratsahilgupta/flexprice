package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
)

// SubscriptionLifecycleConfig holds the schema definition for subscription lifecycle configuration
type SubscriptionLifecycleConfig struct {
	ent.Schema
}

// Mixin of the SubscriptionLifecycleConfig
func (SubscriptionLifecycleConfig) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
		baseMixin.EnvironmentMixin{},
	}
}

// Fields of the SubscriptionLifecycleConfig
func (SubscriptionLifecycleConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Unique().
			Immutable(),
		field.String("key").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Comment("Configuration key (e.g. default_grace_period)"),
		field.String("value").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			NotEmpty().
			Comment("Configuration value"),
		field.String("created_by").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Comment("User ID who created this config"),
		field.String("updated_by").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Comment("User ID who last updated this config"),
	}
}

// Indexes of the SubscriptionLifecycleConfig
func (SubscriptionLifecycleConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "environment_id", "key", "status").
			Unique().
			StorageKey("idx_tenant_env_key_unique").
			Annotations(entsql.IndexWhere("status = 'published'")),
	}
}

// SubscriptionLifecycleConfigAudit holds the schema definition for configuration audit logs
type SubscriptionLifecycleConfigAudit struct {
	ent.Schema
}

// Mixin of the SubscriptionLifecycleConfigAudit
func (SubscriptionLifecycleConfigAudit) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
		baseMixin.EnvironmentMixin{},
	}
}

// Fields of the SubscriptionLifecycleConfigAudit
func (SubscriptionLifecycleConfigAudit) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Unique().
			Immutable(),
		field.String("config_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Comment("ID of the configuration being audited"),
		field.String("key").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Comment("Configuration key that was changed"),
		field.String("previous_value").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			Optional().
			Nillable().
			Comment("Previous configuration value"),
		field.String("new_value").
			SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			NotEmpty().
			Comment("New configuration value"),
		field.Time("changed_at").
			Default(time.Now).
			Comment("When the change was made"),
		field.String("changed_by").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Comment("User ID who made the change"),
	}
}

// Indexes of the SubscriptionLifecycleConfigAudit
func (SubscriptionLifecycleConfigAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "environment_id", "config_id", "changed_at").
			StorageKey("idx_tenant_env_config_time"),
	}
}
