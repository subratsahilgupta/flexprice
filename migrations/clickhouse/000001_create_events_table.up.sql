CREATE TABLE IF NOT EXISTS events (
    -- Core identifiers
    id String, -- Our unique event ID for deduplication
    tenant_id String, -- Identifier for the tenant
    external_customer_id String, -- Identifier for the customer
    
    -- Event metadata
    event_name String, -- Maps to code in Lago
    timestamp DateTime64(3),
    -- Properties as JSON string
    properties String,

    -- Ensure required fields
    CONSTRAINT check_external_customer_id CHECK external_customer_id != '',
    CONSTRAINT check_event_name CHECK event_name != ''
) ENGINE = ReplacingMergeTree(timestamp)
-- Order by clause optimized for our common query patterns
ORDER BY (tenant_id, external_customer_id, event_name, timestamp)
-- Partition by month for better performance
PARTITION BY toYYYYMM(timestamp)
SETTINGS index_granularity = 8192;