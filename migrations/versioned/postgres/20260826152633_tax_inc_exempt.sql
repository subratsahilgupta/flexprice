-- migrate:up
-- Inclusive/exclusive tax behavior + customer tax treatment.
SET lock_timeout = '3s';
SET statement_timeout = '30s';

ALTER TABLE "customers" ADD COLUMN IF NOT EXISTS "tax_treatment" character varying(50) NOT NULL DEFAULT 'taxable';
ALTER TABLE "invoices" ADD COLUMN IF NOT EXISTS "tax_exemption_reason_code" character varying(50) NULL;
ALTER TABLE "tax_associations" ADD COLUMN IF NOT EXISTS "tax_behavior" character varying(50) NULL;

-- tax_applieds.tax_behavior is NOT NULL with no default — a row only exists because tax was
-- applied, and applying it always meant knowing the behavior. Existing rows predate this
-- column, so it is added nullable, backfilled to 'exclusive' (the only behavior that has ever
-- been charged), then locked down.
ALTER TABLE "tax_applieds" ADD COLUMN IF NOT EXISTS "tax_behavior" character varying(50) NULL;
UPDATE "tax_applieds" SET "tax_behavior" = 'exclusive' WHERE "tax_behavior" IS NULL;
ALTER TABLE "tax_applieds" ALTER COLUMN "tax_behavior" SET NOT NULL;

-- migrate:down
SET lock_timeout = '3s';

ALTER TABLE "tax_applieds" DROP COLUMN IF EXISTS "tax_behavior";
ALTER TABLE "tax_associations" DROP COLUMN IF EXISTS "tax_behavior";
ALTER TABLE "invoices" DROP COLUMN IF EXISTS "tax_exemption_reason_code";
ALTER TABLE "customers" DROP COLUMN IF EXISTS "tax_treatment";
