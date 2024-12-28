CREATE TABLE IF NOT EXISTS invoices (
    id VARCHAR(255) PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    customer_id VARCHAR(255) NOT NULL,
    subscription_id VARCHAR(255),
    wallet_id VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'DRAFT',
    currency VARCHAR(10) NOT NULL,
    amount_due NUMERIC(20,8) NOT NULL DEFAULT 0,
    amount_paid NUMERIC(20,8) NOT NULL DEFAULT 0,
    amount_remaining NUMERIC(20,8) NOT NULL DEFAULT 0,
    description TEXT,
    due_date TIMESTAMP WITH TIME ZONE,
    paid_at TIMESTAMP WITH TIME ZONE,
    voided_at TIMESTAMP WITH TIME ZONE,
    finalized_at TIMESTAMP WITH TIME ZONE,
    payment_intent_id VARCHAR(255),
    invoice_pdf_url TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    billing_reason VARCHAR(50),
    metadata JSONB,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_invoices_tenant_customer_status ON invoices(tenant_id, customer_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_invoices_tenant_subscription_status ON invoices(tenant_id, subscription_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_invoices_tenant_wallet_status ON invoices(tenant_id, wallet_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_invoices_tenant_status_due_date ON invoices(tenant_id, status, due_date) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_invoices_tenant_payment_intent ON invoices(tenant_id, payment_intent_id) WHERE deleted_at IS NULL;
