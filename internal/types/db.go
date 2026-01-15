package types

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// LockScope represents the scope of a database advisory lock
type LockScope string

const (
	// LockScopeWallet represents wallet entity locks
	LockScopeWallet LockScope = "wallet"
)

// GenerateLockKey generates a lock key from a scope and parameters.
// Automatically extracts tenant_id and environment_id from context and includes them in the key.
// Reuses the same pattern as idempotency.GenerateKey for consistency.
// The key is a deterministic string that Postgres will hash internally.
func GenerateLockKey(ctx context.Context, scope LockScope, params map[string]interface{}) string {
	// Extract tenant and environment IDs from context
	tenantID := GetTenantID(ctx)
	environmentID := GetEnvironmentID(ctx)

	// Create a merged params map that includes context values
	// User-provided params override context values if same key is provided
	mergedParams := make(map[string]interface{})

	// Add tenant_id from context if present
	if tenantID != "" {
		mergedParams["tenant_id"] = tenantID
	}

	// Add environment_id from context if present
	if environmentID != "" {
		mergedParams["environment_id"] = environmentID
	}

	// Merge user-provided params (these override context values if same key)
	for k, v := range params {
		mergedParams[k] = v
	}

	// Sort params for consistent ordering (same as idempotency generator)
	keys := make([]string, 0, len(mergedParams))
	for k := range mergedParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build string in format: scope:key1=value1:key2=value2:...
	// Same format as idempotency generator, but without hashing
	// (Postgres hashtext() will hash it internally)
	var b strings.Builder
	b.WriteString(string(scope))
	for _, k := range keys {
		b.WriteString(fmt.Sprintf(":%s=%v", k, mergedParams[k]))
	}

	return b.String()
}

// TableName represents a database table name
type TableName string

const (
	TableNameAddons                    TableName = "addons"
	TableNameAddonAssociations         TableName = "addon_associations"
	TableNameAlertLogs                 TableName = "alert_logs"
	TableNameAuths                     TableName = "auths"
	TableNameBillingSequences          TableName = "billing_sequences"
	TableNameConnections               TableName = "connections"
	TableNameCostsheets                TableName = "costsheets"
	TableNameCouponApplications        TableName = "coupon_applications"
	TableNameCouponAssociations        TableName = "coupon_associations"
	TableNameCoupons                   TableName = "coupons"
	TableNameCreditGrantApplications   TableName = "credit_grant_applications"
	TableNameCreditGrants              TableName = "credit_grants"
	TableNameCreditNoteLineItems       TableName = "credit_note_line_items"
	TableNameCreditNotes               TableName = "credit_notes"
	TableNameCustomers                 TableName = "customers"
	TableNameEntitlements              TableName = "entitlements"
	TableNameEntityIntegrationMappings TableName = "entity_integration_mappings"
	TableNameEnvironments              TableName = "environments"
	TableNameFeatures                  TableName = "features"
	TableNameGroups                    TableName = "groups"
	TableNameInvoiceLineItems          TableName = "invoice_line_items"
	TableNameInvoiceSequences          TableName = "invoice_sequences"
	TableNameInvoices                  TableName = "invoices"
	TableNameMeters                    TableName = "meters"
	TableNamePaymentAttempts           TableName = "payment_attempts"
	TableNamePayments                  TableName = "payments"
	TableNamePlans                     TableName = "plans"
	TableNamePriceUnits                TableName = "price_units"
	TableNamePrices                    TableName = "prices"
	TableNameScheduledTasks            TableName = "scheduled_tasks"
	TableNameSecrets                   TableName = "secrets"
	TableNameSettings                  TableName = "settings"
	TableNameSubscriptionLineItems     TableName = "subscription_line_items"
	TableNameSubscriptionPauses        TableName = "subscription_pauses"
	TableNameSubscriptionPhases        TableName = "subscription_phases"
	TableNameSubscriptions             TableName = "subscriptions"
	TableNameTasks                     TableName = "tasks"
	TableNameTaxApplieds               TableName = "tax_applieds"
	TableNameTaxAssociations           TableName = "tax_associations"
	TableNameTaxRates                  TableName = "tax_rates"
	TableNameTenants                   TableName = "tenants"
	TableNameUsers                     TableName = "users"
	TableNameWalletTransactions        TableName = "wallet_transactions"
	TableNameWallets                   TableName = "wallets"
)
