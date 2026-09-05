-- migrate:up
SET lock_timeout = '3s';
SET statement_timeout = '30s';

ALTER TABLE "invoice_line_items" ADD COLUMN IF NOT EXISTS "description" character varying NULL;

-- migrate:down
ALTER TABLE "invoice_line_items" DROP COLUMN IF EXISTS "description";
