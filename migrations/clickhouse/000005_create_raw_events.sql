CREATE TABLE flexprice.raw_events
(
    /* Core identifiers - matching feature_usage */
    id                    String NOT NULL,
    tenant_id             String NOT NULL,
    environment_id        String NOT NULL,
    external_customer_id  String NOT NULL,
    event_name            String NOT NULL DEFAULT '',
    source                Nullable(String),
    
    /* Payload - compressed JSON */
    payload               String CODEC(ZSTD(3)),
    
    /* Flexible fields for quick access */
    field1                Nullable(String),
    field2                Nullable(String),
    field3                Nullable(String),
    field4                Nullable(String),
    field5                Nullable(String),
    field6                Nullable(String),
    field7                Nullable(String),
    field8                Nullable(String),
    field9                Nullable(String),
    field10               Nullable(String),
    
    /* Timestamps - matching feature_usage */
    timestamp             DateTime64(3) NOT NULL,
    ingested_at           DateTime64(3) NOT NULL DEFAULT now64(3),

    /* Deduplication support */
    version               UInt64 NOT NULL DEFAULT toUnixTimestamp64Milli(now64()),
    sign                  Int8 NOT NULL DEFAULT 1,
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(timestamp)  -- MATCHES feature_usage
PRIMARY KEY (tenant_id, environment_id, external_customer_id)  -- ALIGNED with feature_usage
ORDER BY (
    tenant_id, 
    environment_id, 
    external_customer_id,  -- MATCHES feature_usage.customer_id position
    timestamp,
    event_name,
    id
)
SETTINGS index_granularity = 8192;

/* Secondary indexes for comparison queries */
ALTER TABLE flexprice.raw_events
ADD INDEX IF NOT EXISTS bf_event_name   event_name           TYPE bloom_filter(0.01) GRANULARITY 128,
ADD INDEX IF NOT EXISTS bf_source       source               TYPE bloom_filter(0.01) GRANULARITY 128,
ADD INDEX IF NOT EXISTS bf_id           id                   TYPE bloom_filter(0.01) GRANULARITY 128,
ADD INDEX IF NOT EXISTS set_event_name  event_name           TYPE set(0)             GRANULARITY 128;