package subscription

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Plan change request types
type UpgradeSubscriptionRequest struct {
	TargetPlanID         string                  `json:"target_plan_id"`
	ProrationBehavior    types.ProrationBehavior `json:"proration_behavior"`
	EffectiveImmediately bool                    `json:"effective_immediately"`
	Metadata             map[string]string       `json:"metadata,omitempty"`
}

type DowngradeSubscriptionRequest struct {
	TargetPlanID         string                  `json:"target_plan_id"`
	ProrationBehavior    types.ProrationBehavior `json:"proration_behavior"`
	EffectiveAtPeriodEnd bool                    `json:"effective_at_period_end"`
	EffectiveDate        *time.Time              `json:"effective_date,omitempty"`
	Metadata             map[string]string       `json:"metadata,omitempty"`
}

type PreviewPlanChangeRequest struct {
	TargetPlanID      string                  `json:"target_plan_id"`
	ProrationBehavior types.ProrationBehavior `json:"proration_behavior"`
	EffectiveDate     *time.Time              `json:"effective_date,omitempty"`
}

// Plan change response types
type SubscriptionPlanChangeResult struct {
	Subscription    *Subscription     `json:"subscription"`
	Invoice         interface{}       `json:"invoice,omitempty"`  // Will be converted to DTO in handler
	Schedule        interface{}       `json:"schedule,omitempty"` // Will be converted to DTO in handler
	ProrationAmount decimal.Decimal   `json:"proration_amount"`
	ChangeType      string            `json:"change_type"`
	EffectiveDate   time.Time         `json:"effective_date"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type PlanChangePreviewResult struct {
	CurrentAmount   decimal.Decimal `json:"current_amount"`
	NewAmount       decimal.Decimal `json:"new_amount"`
	ProrationAmount decimal.Decimal `json:"proration_amount"`
	EffectiveDate   time.Time       `json:"effective_date"`
	LineItems       []interface{}   `json:"line_items"` // Will be converted to DTO in handler
	Taxes           interface{}     `json:"taxes,omitempty"`
	Coupons         []interface{}   `json:"coupons,omitempty"`
}

// PlanChangeService defines the interface for subscription plan change operations
type SubscriptionChangeService interface {
	// UpgradeSubscription upgrades a subscription to a higher plan immediately
	UpgradeSubscription(ctx context.Context, subscriptionID string, req *UpgradeSubscriptionRequest) (*SubscriptionPlanChangeResult, error)

	// DowngradeSubscription downgrades a subscription to a lower plan
	// Can be immediate or scheduled for period end based on request
	DowngradeSubscription(ctx context.Context, subscriptionID string, req *DowngradeSubscriptionRequest) (*SubscriptionPlanChangeResult, error)

	// PreviewPlanChange previews the impact of a plan change without executing it
	PreviewPlanChange(ctx context.Context, subscriptionID string, req *PreviewPlanChangeRequest) (*PlanChangePreviewResult, error)

	// CancelPendingPlanChange cancels any pending plan changes for a subscription
	CancelPendingPlanChange(ctx context.Context, subscriptionID string) error

	// GetPlanChangeHistory returns the history of plan changes for a subscription
	GetPlanChangeHistory(ctx context.Context, subscriptionID string) ([]*PlanChangeAuditLog, error)
}

// PlanChangeAuditLog represents an audit log entry for plan changes
type PlanChangeAuditLog struct {
	ID              string            `json:"id"`
	SubscriptionID  string            `json:"subscription_id"`
	TenantID        string            `json:"tenant_id"`
	EnvironmentID   string            `json:"environment_id"`
	ChangeType      string            `json:"change_type"`
	SourcePlanID    string            `json:"source_plan_id"`
	TargetPlanID    string            `json:"target_plan_id"`
	ProrationAmount decimal.Decimal   `json:"proration_amount"`
	EffectiveDate   time.Time         `json:"effective_date"`
	Metadata        map[string]string `json:"metadata"`
	CreatedAt       time.Time         `json:"created_at"`
	CreatedBy       string            `json:"created_by"`
}

// PlanChangeValidationResult represents the result of plan change validation
type PlanChangeValidationResult struct {
	Valid               bool     `json:"valid"`
	Errors              []string `json:"errors,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
	CompatibilityIssues []string `json:"compatibility_issues,omitempty"`
}

// PlanChangeContext contains context information for plan change operations
type PlanChangeContext struct {
	SubscriptionID    string
	CurrentPlanID     string
	TargetPlanID      string
	ChangeType        string
	ProrationBehavior string
	EffectiveDate     time.Time
	UserID            string
	TenantID          string
	EnvironmentID     string
}
