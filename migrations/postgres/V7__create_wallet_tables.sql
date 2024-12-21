-- Create wallets table
CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(255) NOT NULL,
    customer_id UUID NOT NULL,
    currency VARCHAR(3) NOT NULL,
    balance DECIMAL(20,4) NOT NULL DEFAULT 0,
    wallet_status VARCHAR(20) NOT NULL DEFAULT 'active',
    metadata JSONB,
    status VARCHAR(20) NOT NULL DEFAULT 'published',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL
);

-- Create wallet transactions table
CREATE TABLE wallet_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(255) NOT NULL,
    wallet_id UUID NOT NULL,
    type VARCHAR(20) NOT NULL,
    amount DECIMAL(20,4) NOT NULL,
    balance_before DECIMAL(20,4) NOT NULL,
    balance_after DECIMAL(20,4) NOT NULL,
    transaction_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    reference_type VARCHAR(50),
    reference_id VARCHAR(255),
    description TEXT,
    metadata JSONB,
    status VARCHAR(20) NOT NULL DEFAULT 'published',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL
);

-- Create indexes
CREATE INDEX idx_wallet_customer ON wallets(customer_id);
CREATE INDEX idx_wallet_tenant ON wallets(tenant_id);

CREATE INDEX idx_transaction_wallet ON wallet_transactions(wallet_id);
CREATE INDEX idx_transaction_reference ON wallet_transactions(reference_type, reference_id);
CREATE INDEX idx_transaction_created_at ON wallet_transactions(created_at DESC);
