package subscription

import (
	"time"
)

// SubscriptionLifecycleConfig represents the configuration for subscription lifecycle management
type SubscriptionLifecycleConfig struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	EnvironmentID string    `json:"environment_id"`
	Key           string    `json:"key"`   // e.g. "default_grace_period", "default_due_date"
	Value         string    `json:"value"` // String representation of the value
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by"` // User ID who created this config
	UpdatedAt     time.Time `json:"updated_at"`
	UpdatedBy     string    `json:"updated_by"` // User ID who last updated this config
	Status        string    `json:"status"`
}

// SubscriptionLifecycleConfigAudit represents an audit log entry for configuration changes
type SubscriptionLifecycleConfigAudit struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	EnvironmentID string    `json:"environment_id"`
	ConfigID      string    `json:"config_id"`
	Key           string    `json:"key"`
	PreviousValue string    `json:"previous_value"`
	NewValue      string    `json:"new_value"`
	ChangedAt     time.Time `json:"changed_at"`
	ChangedBy     string    `json:"changed_by"` // User ID who made the change
}

const (
	// Configuration keys
	ConfigKeyDefaultGracePeriod = "default_grace_period"
	ConfigKeyDefaultDueDate     = "default_due_date"

	// Default values
	DefaultGracePeriodDays = 1
	DefaultDueDateDays     = 1
)
