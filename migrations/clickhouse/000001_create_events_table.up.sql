CREATE TABLE IF NOT EXISTS events (
    -- Core identifiers
    id String,
    tenant_id String,
    external_customer_id String,
    customer_id String,

    -- Event metadata
    event_name String,
    source String,
    timestamp DateTime64(3),
    ingested_at DateTime64(3) DEFAULT now(),
    -- Properties as JSON string
    properties String,

    -- Ensure required fields
    CONSTRAINT check_event_name CHECK (event_name != ''),
    CONSTRAINT check_tenant_id CHECK (tenant_id != ''),
    CONSTRAINT check_event_id CHECK (id != '')
) ENGINE = ReplacingMergeTree(timestamp)
PARTITION BY toYYYYMM(timestamp)
ORDER BY (id, tenant_id, external_customer_id, customer_id, event_name, timestamp)
SETTINGS index_granularity = 8192;