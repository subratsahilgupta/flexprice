#!/usr/bin/env bash
set -euo pipefail

# ---------------------------
# USER CONFIG
# ---------------------------
# Copy raw_events from one environment_id to another (same table).
# Source rows: environment_id = SRC_ENV_ID
# Inserted rows: same data but environment_id = DST_ENV_ID (for dual-write / migration).

CH_HOST="${CH_HOST:-127.0.0.1}"
CH_PORT="${CH_PORT:-9000}"
CH_USER="${CH_USER:-default}"
CH_PASSWORD="${CH_PASSWORD:-}"
CH_DB="${CH_DB:-flexprice}"

TABLE="${TABLE:-raw_events}"
TS_COL="${TS_COL:-timestamp}"

# Source environment (rows we read from)
SRC_ENV_ID="${SRC_ENV_ID:-env_01KF5GXB8X2TYQHSVE4YVZYCN8}"
# Destination environment (value we write into the new rows)
DST_ENV_ID="${DST_ENV_ID:-env_01KG4E6FR5YCNW0742N6CA1YD1}"

# Optional: restrict to one tenant (uses primary key, much more efficient)
TENANT_ID="${TENANT_ID:-}"

# Timezone for day boundaries (must match how timestamp is stored; default UTC)
TIMEZONE="${TIMEZONE:-UTC}"
# Brief sleep after INSERT before verification count (ReplacingMergeTree parts visibility)
VERIFY_SLEEP_SEC="${VERIFY_SLEEP_SEC:-5}"

START_DATE="${START_DATE:-2026-01-01}"
END_DATE_EXCL="${END_DATE_EXCL:-2026-01-29}"

PARALLEL="${PARALLEL:-10}"
MAX_RETRIES="${MAX_RETRIES:-8}"
BASE_BACKOFF_SEC="${BASE_BACKOFF_SEC:-5}"

CONNECT_TIMEOUT_SEC="${CONNECT_TIMEOUT_SEC:-10}"
SEND_TIMEOUT_SEC="${SEND_TIMEOUT_SEC:-30}"
RECEIVE_TIMEOUT_SEC="${RECEIVE_TIMEOUT_SEC:-30}"
MAX_EXEC_TIME="${MAX_EXEC_TIME:-0}"

LOG_DIR="${LOG_DIR:-./logs/copy_raw_events_env}"
mkdir -p "$LOG_DIR"

# ---------------------------
# CLICKHOUSE CLIENT WRAPPER
# ---------------------------
ch() {
  clickhouse client \
    --host "$CH_HOST" --port "$CH_PORT" \
    --user "$CH_USER" --password "$CH_PASSWORD" \
    --database "$CH_DB" \
    --connect_timeout "$CONNECT_TIMEOUT_SEC" \
    --send_timeout "$SEND_TIMEOUT_SEC" \
    --receive_timeout "$RECEIVE_TIMEOUT_SEC" \
    --multiquery \
    --format=TSV \
    "$@"
}

# ---------------------------
# HELPERS
# ---------------------------
# macOS: install coreutils and use gdate. Linux: can use date -d.
date_add() { gdate -d "$1 +1 day" +"%Y-%m-%d"; }

# Optional tenant filter fragment (uses primary key when set)
tenant_filter() {
  if [[ -n "${TENANT_ID:-}" ]]; then
    echo "AND tenant_id = '${TENANT_ID}'"
  else
    echo ""
  fi
}

# Day bounds in configured timezone (consistent with how timestamp is stored)
day_where() {
  local day="$1"
  echo "${TS_COL} >= toDateTime64('${day} 00:00:00', 3, '${TIMEZONE}')
      AND ${TS_COL} <  toDateTime64('${day} 00:00:00', 3, '${TIMEZONE}') + INTERVAL 1 DAY"
}

# Check if destination env already has rows for this day (idempotency)
# Use FINAL to get deduplicated count from ReplacingMergeTree
exists_day_in_dst() {
  local day="$1"
  local tf
  tf="$(tenant_filter)"
  local dw
  dw="$(day_where "$day")"
  ch --query "
    SELECT count()
    FROM ${TABLE} FINAL
    WHERE environment_id = '${DST_ENV_ID}'
      AND ${dw}
      ${tf}
  " 2>/dev/null | tr -d '\r'
}

copy_day() {
  local day="$1"
  local log="$LOG_DIR/${day}.log"

  echo "[$(gdate -Iseconds)] Starting ${day} (${SRC_ENV_ID} -> ${DST_ENV_ID})" | tee -a "$log"

  local already
  already="$(exists_day_in_dst "$day" || echo "0")"
  already="$(echo "$already" | tr -d '\r\n ')"
  echo "[$(gdate -Iseconds)] DEBUG: already='$already' for ${day}" | tee -a "$log"
  if [[ -n "$already" && "$already" != "0" ]]; then
    echo "[$(gdate -Iseconds)] SKIP ${day} (dst env already has ${already} rows)" | tee -a "$log"
    return 0
  fi

  # Check how many rows exist in source env for this day (debug)
  local dw
  dw="$(day_where "$day")"
  local tf
  tf="$(tenant_filter)"
  
  local src_count
  src_count="$(ch --query "
    SELECT count()
    FROM ${TABLE}
    WHERE environment_id = '${SRC_ENV_ID}'
      AND ${dw}
      ${tf}
  " 2>/dev/null | tr -d '\r\n ')"
  echo "[$(gdate -Iseconds)] Source rows for ${day}: ${src_count}" | tee -a "$log"
  
  if [[ "${src_count}" == "0" ]]; then
    echo "[$(gdate -Iseconds)] SKIP ${day} (no source rows)" | tee -a "$log"
    return 0
  fi
  
  # Test: verify basic SELECT works
  echo "[$(gdate -Iseconds)] Testing: Can we select rows with LIMIT 10?" | tee -a "$log"
  local test_basic
  test_basic="$(ch --query "
    SELECT count()
    FROM ${TABLE}
    WHERE environment_id = '${SRC_ENV_ID}'
      AND ${dw}
      ${tf}
    LIMIT 10
  " 2>&1 | tee -a "$log" | tail -1 | tr -d '\r\n ')"
  echo "[$(gdate -Iseconds)] Basic SELECT LIMIT 10 returned: ${test_basic}" | tee -a "$log"
  
  # Test: verify we can select specific columns
  echo "[$(gdate -Iseconds)] Testing: Can we select specific columns?" | tee -a "$log"
  local test_columns
  test_columns="$(ch --query "
    SELECT count()
    FROM (
      SELECT id, tenant_id, timestamp
      FROM ${TABLE}
      WHERE environment_id = '${SRC_ENV_ID}'
        AND ${dw}
        ${tf}
      LIMIT 10
    )
  " 2>&1 | tee -a "$log" | tail -1 | tr -d '\r\n ')"
  echo "[$(gdate -Iseconds)] Column SELECT returned: ${test_columns}" | tee -a "$log"

  local attempt=1
  while (( attempt <= MAX_RETRIES )); do
    echo "[$(gdate -Iseconds)] Attempt ${attempt}/${MAX_RETRIES} for ${day}" | tee -a "$log"

    # Insert into same table: select from source env, replace environment_id with dest env.
    # Explicit column list to set environment_id to DST_ENV_ID.
    
    # Build the query as a variable for debugging
    # NOTE: We use a subquery to avoid ClickHouse optimization issues when overriding environment_id.
    # We explicitly list all columns EXCEPT version, which will use DEFAULT value.
    # This ensures copied rows get a new version timestamp and are treated as newer by ReplacingMergeTree.
    local query="INSERT INTO ${TABLE} (
  id,
  tenant_id,
  environment_id,
  external_customer_id,
  event_name,
  source,
  payload,
  field1,
  field2,
  field3,
  field4,
  field5,
  field6,
  field7,
  field8,
  field9,
  field10,
  timestamp,
  ingested_at,
  sign
)
SELECT
  id,
  tenant_id,
  '${DST_ENV_ID}' AS environment_id,
  external_customer_id,
  event_name,
  source,
  payload,
  field1,
  field2,
  field3,
  field4,
  field5,
  field6,
  field7,
  field8,
  field9,
  field10,
  timestamp,
  ingested_at,
  sign
FROM (
  SELECT
    id,
    tenant_id,
    environment_id,
    external_customer_id,
    event_name,
    source,
    payload,
    field1,
    field2,
    field3,
    field4,
    field5,
    field6,
    field7,
    field8,
    field9,
    field10,
    timestamp,
    ingested_at,
    sign
  FROM ${TABLE}
  WHERE environment_id = '${SRC_ENV_ID}'
    AND ${dw}
    ${tf}
)
SETTINGS
  max_execution_time = ${MAX_EXEC_TIME},
  max_threads = 0,
  max_insert_threads = 0"
    
    echo "[$(gdate -Iseconds)] Executing query:" | tee -a "$log"
    echo "$query" >> "$log"
    echo "---" >> "$log"
    
    local insert_result
    insert_result=$(echo "$query" | ch 2>&1)
    local insert_exit=$?
    echo "$insert_result" >> "$log"
    echo "[$(gdate -Iseconds)] INSERT exit code: $insert_exit" | tee -a "$log"
    
    if [[ $insert_exit -eq 0 ]]; then
      [[ -n "${VERIFY_SLEEP_SEC:-}" && "${VERIFY_SLEEP_SEC:-0}" -gt 0 ]] && sleep "${VERIFY_SLEEP_SEC}"
      local cnt
      cnt="$(exists_day_in_dst "$day" || echo "0")"
      echo "[$(gdate -Iseconds)] DONE ${day} (dst env rows=${cnt})" | tee -a "$log"
      return 0
    else
      echo "[$(gdate -Iseconds)] INSERT failed with output above" | tee -a "$log"
    fi

    local sleep_for=$(( BASE_BACKOFF_SEC * attempt ))
    echo "[$(gdate -Iseconds)] FAIL ${day} attempt ${attempt}. Sleeping ${sleep_for}s then retry..." | tee -a "$log"
    sleep "$sleep_for"
    attempt=$(( attempt + 1 ))
  done

  echo "[$(gdate -Iseconds)] ERROR: Giving up on ${day} after ${MAX_RETRIES} attempts" | tee -a "$log"
  return 1
}
export -f copy_day
export -f exists_day_in_dst
export -f ch
export -f date_add
export -f tenant_filter
export -f day_where

# ---------------------------
# MAIN: build list of days and run parallel
# ---------------------------
days=()
d="$START_DATE"
while [[ "$d" != "$END_DATE_EXCL" ]]; do
  days+=("$d")
  d="$(date_add "$d")"
done

echo "Copy raw_events: ${SRC_ENV_ID} -> ${DST_ENV_ID}"
[[ -n "${TENANT_ID:-}" ]] && echo "Tenant filter: ${TENANT_ID}"
echo "Day bounds timezone: ${TIMEZONE}"
echo "Will process ${#days[@]} days from ${START_DATE} to $(gdate -d "${END_DATE_EXCL} -1 day" +%Y-%m-%d 2>/dev/null || echo "${END_DATE_EXCL}(exclusive)")"
echo "Parallelism: ${PARALLEL}"
echo "Logs: ${LOG_DIR}"

export PATH
export CH_HOST CH_PORT CH_USER CH_PASSWORD CH_DB
export TABLE TS_COL SRC_ENV_ID DST_ENV_ID
export TENANT_ID TIMEZONE VERIFY_SLEEP_SEC
export MAX_RETRIES BASE_BACKOFF_SEC
export CONNECT_TIMEOUT_SEC SEND_TIMEOUT_SEC RECEIVE_TIMEOUT_SEC MAX_EXEC_TIME
export LOG_DIR

if command -v parallel &> /dev/null; then
  export -f ch exists_day_in_dst date_add copy_day tenant_filter day_where
  printf "%s\n" "${days[@]}" | parallel -j "$PARALLEL" copy_day \
    1> "$LOG_DIR/run.stdout.log" 2> "$LOG_DIR/run.stderr.log"
else
  for day in "${days[@]}"; do
    copy_day "$day"
  done
fi

echo "All done. Check $LOG_DIR for per-day logs."

: '
==================================================
USAGE EXAMPLES
==================================================

Copy raw_events from env_01KF5GXB8X2TYQHSVE4YVZYCN8 to env_01KG4E6FR5YCNW0742N6CA1YD1
for a single day:
  source .env.backfill && START_DATE=2026-01-29 END_DATE_EXCL=2026-01-30 ./copy_raw_events_env.sh

For a date range (per-day, parallel):
  source .env.backfill && START_DATE=2026-01-01 END_DATE_EXCL=2026-01-30 ./copy_raw_events_env.sh

With tenant filter (uses primary key, more efficient):
  source .env.backfill && TENANT_ID=tenant_xxx START_DATE=2026-01-29 END_DATE_EXCL=2026-01-30 ./copy_raw_events_env.sh

Custom env IDs:
  source .env.backfill && SRC_ENV_ID=env_old DST_ENV_ID=env_new START_DATE=2026-01-01 END_DATE_EXCL=2026-02-01 ./copy_raw_events_env.sh

If timestamps are stored in a specific timezone (default UTC):
  source .env.backfill && TIMEZONE=Asia/Kolkata START_DATE=2026-01-01 END_DATE_EXCL=2026-01-30 ./copy_raw_events_env.sh

Lower parallelism for very large days:
  source .env.backfill && PARALLEL=4 START_DATE=2026-01-01 END_DATE_EXCL=2026-01-30 ./copy_raw_events_env.sh

==================================================
CONFIGURABLE ENVIRONMENT VARIABLES
==================================================

ClickHouse: CH_HOST, CH_PORT, CH_USER, CH_PASSWORD, CH_DB
Table:      TABLE (default: raw_events), TS_COL (default: timestamp)
Envs:       SRC_ENV_ID (source), DST_ENV_ID (destination)
Filter:     TENANT_ID (optional; when set, uses primary key for efficiency)
Timezone:   TIMEZONE (day boundaries; default UTC; set if timestamp is stored in another TZ)
Dates:      START_DATE, END_DATE_EXCL (exclusive)
Execution:  PARALLEL, MAX_RETRIES, BASE_BACKOFF_SEC, MAX_EXEC_TIME, LOG_DIR
            VERIFY_SLEEP_SEC (seconds to wait after INSERT before count; default 2)
'
