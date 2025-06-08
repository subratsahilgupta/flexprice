package creditgrantapplication

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type CreditGrantApplication struct {
	ID             string `db:"id" json:"id"`
	CreditGrantID  string `db:"credit_grant_id" json:"credit_grant_id"`
	SubscriptionID string `db:"subscription_id" json:"subscription_id"`

	// Timing
	ScheduledFor time.Time  `db:"scheduled_for" json:"scheduled_for"`
	AppliedAt    *time.Time `db:"applied_at" json:"applied_at,omitempty"`

	// Billing period context
	BillingPeriodStart time.Time `db:"billing_period_start" json:"billing_period_start"`
	BillingPeriodEnd   time.Time `db:"billing_period_end" json:"billing_period_end"`

	// Application details
	ApplicationStatus types.ApplicationStatus `db:"application_status" json:"application_status"`
	AmountApplied     decimal.Decimal         `db:"amount_applied" json:"amount_applied"`
	Currency          string                  `db:"currency" json:"currency"`

	// Context
	ApplicationReason               string `db:"application_reason" json:"application_reason"`
	SubscriptionStatusAtApplication string `db:"subscription_status_at_application" json:"subscription_status_at_application"`

	// Prorating
	IsProrated       bool             `db:"is_prorated" json:"is_prorated"`
	ProrationFactor  *decimal.Decimal `db:"proration_factor" json:"proration_factor,omitempty"`
	FullPeriodAmount *decimal.Decimal `db:"full_period_amount" json:"full_period_amount,omitempty"`

	// Retry handling
	RetryCount    int        `db:"retry_count" json:"retry_count"`
	FailureReason *string    `db:"failure_reason" json:"failure_reason,omitempty"`
	NextRetryAt   *time.Time `db:"next_retry_at" json:"next_retry_at,omitempty"`

	Metadata types.Metadata `db:"metadata" json:"metadata,omitempty"`

	types.BaseModel
}
