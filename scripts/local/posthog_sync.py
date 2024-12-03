import clickhouse_connect
import time
import os
from dotenv import load_dotenv

# Load environment variables from .env file in the current directory
env_file_path = os.path.join(os.path.dirname(__file__), '.env')
load_dotenv(env_file_path)

# Extract ClickHouse credentials
CLICKHOUSE_HOST = os.getenv('CLICKHOUSE_HOST', '127.0.0.1')
CLICKHOUSE_PORT = os.getenv('CLICKHOUSE_PORT', '9000')
CLICKHOUSE_USER = os.getenv('CLICKHOUSE_USER', 'default')
CLICKHOUSE_PASSWORD = os.getenv('CLICKHOUSE_PASSWORD', 'default')
CLICKHOUSE_DB = os.getenv('CLICKHOUSE_DB', 'flexprice')

# Extract S3 credentials
S3_URL = os.getenv('S3_URL', 'https://fp-posthog-sync.s3.amazonaws.com/posthog-events/*.parquet')
S3_ACCESS_KEY = os.getenv('S3_ACCESS_KEY', '')
S3_SECRET_KEY = os.getenv('S3_SECRET_KEY', '')

# ClickHouse connection setup
client = clickhouse_connect.get_client(
    host=CLICKHOUSE_HOST,
    port=int(CLICKHOUSE_PORT),
    username=CLICKHOUSE_USER,
    password=CLICKHOUSE_PASSWORD,
    database=CLICKHOUSE_DB
)

# S3 import SQL query
query = f"""
INSERT INTO flexprice.events (
    id, tenant_id, external_customer_id, customer_id, event_name, source, timestamp, ingested_at, properties
)
WITH 
    parsedData AS (
        SELECT 
            uuid,
            event,
            JSONExtractRaw(properties) AS properties,
            timestamp,
            distinct_id
        FROM s3('{S3_URL}',
               '{S3_ACCESS_KEY}',
               '{S3_SECRET_KEY}',
               'Parquet',
               'uuid String, event String, properties String, timestamp DateTime64(3), distinct_id String'
        )
    )
SELECT
    uuid AS id,
    'flexprice' AS tenant_id,
    distinct_id AS external_customer_id,
    'hemabh' AS customer_id,
    event AS event_name,
    'posthog' AS source,
    timestamp AS timestamp,
    toDateTime64(now(), 3) AS ingested_at,
    properties -- JSONExtractRaw retains dynamic JSON format
FROM parsedData
WHERE event != ''
AND distinct_id != ''
"""

try:
    print("Inserting into event_raw table...")
    
    # Start time
    start_time = time.time()
    
    # Execute the query
    client.query(query, settings={"log_queries": 1, "send_progress_in_http_headers": 1})
    
    # End time and calculate total time
    end_time = time.time()
    total_time = end_time - start_time

    print("Data copied completed successfully.")
    print(f"Total time taken: {total_time:.2f} seconds")

except Exception as e:
    print("An error occurred during import:", e)

finally:
    client.close()
