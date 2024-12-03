CREATE TABLE IF NOT EXISTS prices (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id VARCHAR(255) NOT NULL,
    amount INTEGER NOT NULL,
    currency VARCHAR(3) NOT NULL,
    external_id VARCHAR(255),
    external_source VARCHAR(255),
    billing_period VARCHAR(20) NOT NULL,
    billing_period_count INTEGER NOT NULL,
    billing_model VARCHAR(20) NOT NULL,
    billing_cadence VARCHAR(20) NOT NULL,
    billing_country_code VARCHAR(3),
    tier_mode VARCHAR(20),
    tiers JSONB,
    transform JSONB,
    lookup_key VARCHAR(255) NOT NULL,
    description TEXT,
    metadata JSONB,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL
);

-- Add indexes
CREATE INDEX idx_prices_tenant_id ON prices(tenant_id);
CREATE INDEX idx_prices_lookup_key ON prices(lookup_key);
CREATE INDEX idx_prices_external_id ON prices(external_id); 