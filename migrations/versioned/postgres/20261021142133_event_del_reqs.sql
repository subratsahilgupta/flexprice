-- migrate:up
-- Bulk event deletion (FLE-687): one row per customer per in-flight deletion.
-- Brand-new table, so the partial unique index builds instantly inside this
-- ordinary transactional migration. CONCURRENTLY only matters for an index on an
-- already-populated table and would require a single-statement transaction:false file.
-- The index is the fence: it admits at most one DELETING row per customer, which is
-- what stops two deletions overlapping on the same usage.
SET lock_timeout = '3s';
SET statement_timeout = '30s';

CREATE TABLE IF NOT EXISTS "event_deletion_requests" ("id" character varying(50) NOT NULL, "tenant_id" character varying(50) NOT NULL, "status" character varying(20) NOT NULL DEFAULT 'published', "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "created_by" character varying NULL, "updated_by" character varying NULL, "environment_id" character varying(50) NULL DEFAULT '', "request_id" character varying(128) NOT NULL, "external_customer_id" character varying(255) NOT NULL, "period_start" timestamptz NOT NULL, "period_end" timestamptz NOT NULL, "deletion_status" character varying(20) NOT NULL, "started_at" timestamptz NOT NULL, "completed_at" timestamptz NULL, "failed_at" timestamptz NULL, "failure_reason" text NULL, "skip_payload" boolean NOT NULL DEFAULT false, PRIMARY KEY ("id"));
CREATE UNIQUE INDEX "idx_event_deletion_request_in_progress" ON "event_deletion_requests" ("tenant_id", "environment_id", "external_customer_id") WHERE (((status)::text = 'published'::text) AND ((deletion_status)::text = 'DELETING'::text));

-- migrate:down
DROP TABLE IF EXISTS "event_deletion_requests";
