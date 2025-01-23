package config

import "github.com/flexprice/flexprice/internal/types"

// Webhook represents the configuration for the webhook system
type Webhook struct {
	Enabled bool                           `mapstructure:"enabled"`
	Topic   string                         `mapstructure:"topic" default:"webhooks"`
	PubSub  types.PubSubType               `mapstructure:"pubsub" default:"memory"`
	Tenants map[string]TenantWebhookConfig `mapstructure:"tenants"`
}

// TenantWebhookConfig represents webhook configuration for a specific tenant
type TenantWebhookConfig struct {
	Endpoint       string            `mapstructure:"endpoint"`
	Headers        map[string]string `mapstructure:"headers"`
	Enabled        bool              `mapstructure:"enabled"`
	ExcludedEvents []string          `mapstructure:"excluded_events"`
}
