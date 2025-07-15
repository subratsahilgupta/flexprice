package subscription

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

type Repository interface {
	Create(ctx context.Context, subscription *Subscription) error
	Get(ctx context.Context, id string) (*Subscription, error)
	Update(ctx context.Context, subscription *Subscription) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter *types.SubscriptionFilter) ([]*Subscription, error)
	Count(ctx context.Context, filter *types.SubscriptionFilter) (int, error)
	ListAll(ctx context.Context, filter *types.SubscriptionFilter) ([]*Subscription, error)
	ListAllTenant(ctx context.Context, filter *types.SubscriptionFilter) ([]*Subscription, error)
	ListByCustomerID(ctx context.Context, customerID string) ([]*Subscription, error)
	ListByIDs(ctx context.Context, subscriptionIDs []string) ([]*Subscription, error)
	CreateWithLineItems(ctx context.Context, subscription *Subscription, items []*SubscriptionLineItem) error
	GetWithLineItems(ctx context.Context, id string) (*Subscription, []*SubscriptionLineItem, error)

	// Pause-related methods
	CreatePause(ctx context.Context, pause *SubscriptionPause) error
	GetPause(ctx context.Context, id string) (*SubscriptionPause, error)
	UpdatePause(ctx context.Context, pause *SubscriptionPause) error
	ListPauses(ctx context.Context, subscriptionID string) ([]*SubscriptionPause, error)
	GetWithPauses(ctx context.Context, id string) (*Subscription, []*SubscriptionPause, error)
}

// SubscriptionScheduleRepository provides access to the subscription schedule store
type SubscriptionScheduleRepository interface {
	// Create creates a new subscription schedule
	Create(ctx context.Context, schedule *SubscriptionSchedule) error

	// Get retrieves a subscription schedule by ID
	Get(ctx context.Context, id string) (*SubscriptionSchedule, error)

	// GetBySubscriptionID gets a schedule for a subscription if it exists
	GetBySubscriptionID(ctx context.Context, subscriptionID string) (*SubscriptionSchedule, error)

	// Update updates a subscription schedule
	Update(ctx context.Context, schedule *SubscriptionSchedule) error

	// Delete deletes a subscription schedule
	Delete(ctx context.Context, id string) error

	// ListPhases lists all phases for a subscription schedule
	ListPhases(ctx context.Context, scheduleID string) ([]*SchedulePhase, error)

	// CreatePhase creates a new subscription schedule phase
	CreatePhase(ctx context.Context, phase *SchedulePhase) error

	// GetPhase gets a subscription schedule phase by ID
	GetPhase(ctx context.Context, id string) (*SchedulePhase, error)

	// UpdatePhase updates a subscription schedule phase
	UpdatePhase(ctx context.Context, phase *SchedulePhase) error

	// DeletePhase deletes a subscription schedule phase
	DeletePhase(ctx context.Context, id string) error

	// CreateWithPhases creates a schedule with all its phases in one transaction
	CreateWithPhases(ctx context.Context, schedule *SubscriptionSchedule, phases []*SchedulePhase) error
}

// SubscriptionLifecycleConfigRepository provides access to subscription lifecycle configuration
type SubscriptionLifecycleConfigRepository interface {
	// GetConfig retrieves a configuration value by key
	GetConfig(ctx context.Context, tenantID, environmentID, key string) (*SubscriptionLifecycleConfig, error)

	// SetConfig creates or updates a configuration value
	SetConfig(ctx context.Context, config *SubscriptionLifecycleConfig) error

	// GetConfigAudit retrieves audit logs for a configuration key
	GetConfigAudit(ctx context.Context, tenantID, environmentID, key string) ([]*SubscriptionLifecycleConfigAudit, error)

	// CreateConfigAudit creates a new audit log entry
	CreateConfigAudit(ctx context.Context, audit *SubscriptionLifecycleConfigAudit) error
}
