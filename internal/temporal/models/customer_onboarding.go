package models

import (
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// WorkflowStatus represents the status of a workflow execution
type WorkflowStatus string

const (
	WorkflowStatusCompleted WorkflowStatus = "completed"
	WorkflowStatusFailed    WorkflowStatus = "failed"
)

// WorkflowResourceType represents the type of resource created by a workflow action
type WorkflowResourceType string

const (
	WorkflowResourceTypeCustomer     WorkflowResourceType = "customer"
	WorkflowResourceTypeWallet       WorkflowResourceType = "wallet"
	WorkflowResourceTypeSubscription WorkflowResourceType = "subscription"
)

// CustomerOnboardingWorkflowInput represents the input for the customer onboarding workflow
type CustomerOnboardingWorkflowInput struct {
	CustomerID         string         `json:"customer_id,omitempty"`          // Optional - provided when customer exists
	ExternalCustomerID string         `json:"external_customer_id,omitempty"` // Optional - used for auto-creation
	EventTimestamp     *time.Time     `json:"event_timestamp,omitempty"`      // Optional - timestamp of the triggering event
	TenantID           string         `json:"tenant_id" validate:"required"`
	EnvironmentID      string         `json:"environment_id" validate:"required"`
	UserID             string         `json:"user_id,omitempty"`
	WorkflowConfig     WorkflowConfig `json:"workflow_config" validate:"required"`
}

// Validate validates the customer onboarding workflow input
func (c *CustomerOnboardingWorkflowInput) Validate() error {
	// Either CustomerID or ExternalCustomerID must be provided
	if c.CustomerID == "" && c.ExternalCustomerID == "" {
		return ierr.NewError("either customer_id or external_customer_id is required").
			WithHint("Please provide either an existing customer ID or external customer ID for auto-creation").
			Mark(ierr.ErrValidation)
	}
	if c.TenantID == "" {
		return ierr.NewError("tenant_id is required").
			WithHint("Please provide a valid tenant ID").
			Mark(ierr.ErrValidation)
	}
	if c.EnvironmentID == "" {
		return ierr.NewError("environment_id is required").
			WithHint("Please provide a valid environment ID").
			Mark(ierr.ErrValidation)
	}
	// user_id is optional - will be provided from workflow config's default_user_id
	return c.WorkflowConfig.Validate()
}

// CustomerOnboardingWorkflowResult represents the result of the customer onboarding workflow
type CustomerOnboardingWorkflowResult struct {
	CustomerID      string                           `json:"customer_id"`
	Status          WorkflowStatus                   `json:"status"`
	CompletedAt     time.Time                        `json:"completed_at"`
	ActionsExecuted int                              `json:"actions_executed"`
	Results         []CustomerOnboardingActionResult `json:"results"`
	ErrorSummary    *string                          `json:"error_summary,omitempty"`
}

// CustomerOnboardingActionResult represents the result of a single action in the workflow
type CustomerOnboardingActionResult struct {
	ActionType   WorkflowAction       `json:"action_type"`
	ActionIndex  int                  `json:"action_index"`
	Status       WorkflowStatus       `json:"status"`
	ResourceID   string               `json:"resource_id,omitempty"`
	ResourceType WorkflowResourceType `json:"resource_type,omitempty"`
	Error        *string              `json:"error,omitempty"`
}

// CreateCustomerActivityInput represents the input for the create customer activity
type CreateCustomerActivityInput struct {
	ExternalID    string `json:"external_id" validate:"required"`
	Name          string `json:"name" validate:"required"`
	Email         string `json:"email,omitempty"`
	TenantID      string `json:"tenant_id" validate:"required"`
	EnvironmentID string `json:"environment_id" validate:"required"`
	UserID        string `json:"user_id,omitempty"`
}

// Validate validates the create customer activity input
func (c *CreateCustomerActivityInput) Validate() error {
	if c.ExternalID == "" {
		return ierr.NewError("external_id is required").
			WithHint("Please provide a valid external customer ID").
			Mark(ierr.ErrValidation)
	}
	if c.Name == "" {
		return ierr.NewError("name is required").
			WithHint("Please provide a customer name").
			Mark(ierr.ErrValidation)
	}
	if c.TenantID == "" {
		return ierr.NewError("tenant_id is required").
			WithHint("Please provide a valid tenant ID").
			Mark(ierr.ErrValidation)
	}
	if c.EnvironmentID == "" {
		return ierr.NewError("environment_id is required").
			WithHint("Please provide a valid environment ID").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// CreateCustomerActivityResult represents the result of the create customer activity
type CreateCustomerActivityResult struct {
	CustomerID string `json:"customer_id"`
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
}

// CreateWalletActivityInput represents the input for the create wallet activity
type CreateWalletActivityInput struct {
	CustomerID    string                    `json:"customer_id" validate:"required"`
	TenantID      string                    `json:"tenant_id" validate:"required"`
	EnvironmentID string                    `json:"environment_id" validate:"required"`
	UserID        string                    `json:"user_id,omitempty"`
	WalletConfig  *CreateWalletActionConfig `json:"wallet_config" validate:"required"`
}

// Validate validates the create wallet activity input
func (c *CreateWalletActivityInput) Validate() error {
	if c.CustomerID == "" {
		return ierr.NewError("customer_id is required").
			WithHint("Please provide a valid customer ID").
			Mark(ierr.ErrValidation)
	}
	if c.TenantID == "" {
		return ierr.NewError("tenant_id is required").
			WithHint("Please provide a valid tenant ID").
			Mark(ierr.ErrValidation)
	}
	if c.EnvironmentID == "" {
		return ierr.NewError("environment_id is required").
			WithHint("Please provide a valid environment ID").
			Mark(ierr.ErrValidation)
	}
	if c.WalletConfig == nil {
		return ierr.NewError("wallet_config is required").
			WithHint("Please provide wallet configuration").
			Mark(ierr.ErrValidation)
	}
	return c.WalletConfig.Validate()
}

// CreateWalletActivityResult represents the result of the create wallet activity
type CreateWalletActivityResult struct {
	WalletID       string             `json:"wallet_id"`
	CustomerID     string             `json:"customer_id"`
	Currency       string             `json:"currency"`
	ConversionRate string             `json:"conversion_rate"`
	WalletType     types.WalletType   `json:"wallet_type"`
	Status         types.WalletStatus `json:"status"`
}

// CreateSubscriptionActivityInput represents the input for the create subscription activity
type CreateSubscriptionActivityInput struct {
	CustomerID         string                          `json:"customer_id" validate:"required"`
	EventTimestamp     *time.Time                      `json:"event_timestamp,omitempty"` // Optional - timestamp of the triggering event
	TenantID           string                          `json:"tenant_id" validate:"required"`
	EnvironmentID      string                          `json:"environment_id" validate:"required"`
	UserID             string                          `json:"user_id,omitempty"`
	SubscriptionConfig *CreateSubscriptionActionConfig `json:"subscription_config" validate:"required"`
}

// Validate validates the create subscription activity input
func (c *CreateSubscriptionActivityInput) Validate() error {
	if c.CustomerID == "" {
		return ierr.NewError("customer_id is required").
			WithHint("Please provide a valid customer ID").
			Mark(ierr.ErrValidation)
	}
	if c.TenantID == "" {
		return ierr.NewError("tenant_id is required").
			WithHint("Please provide a valid tenant ID").
			Mark(ierr.ErrValidation)
	}
	if c.EnvironmentID == "" {
		return ierr.NewError("environment_id is required").
			WithHint("Please provide a valid environment ID").
			Mark(ierr.ErrValidation)
	}
	if c.SubscriptionConfig == nil {
		return ierr.NewError("subscription_config is required").
			WithHint("Please provide subscription configuration").
			Mark(ierr.ErrValidation)
	}
	return c.SubscriptionConfig.Validate()
}

// CreateSubscriptionActivityResult represents the result of the create subscription activity
type CreateSubscriptionActivityResult struct {
	SubscriptionID string                   `json:"subscription_id"`
	CustomerID     string                   `json:"customer_id"`
	PlanID         string                   `json:"plan_id"`
	Currency       string                   `json:"currency"`
	Status         types.SubscriptionStatus `json:"status"`
	BillingCycle   types.BillingCycle       `json:"billing_cycle"`
	StartDate      *time.Time               `json:"start_date"`
}
