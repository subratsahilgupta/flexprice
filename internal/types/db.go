package types

// LockScope represents the scope of a database advisory lock
type LockScope string

const (
	// LockScopeWallet represents wallet entity locks
	LockScopeWallet LockScope = "wallet"
)

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
