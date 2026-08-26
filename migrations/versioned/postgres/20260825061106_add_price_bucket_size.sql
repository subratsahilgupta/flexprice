-- migrate:up
-- prices.bucket_size arrived on develop in c0b294ddf, after India prod's schema
-- was captured. GCP staging already has it; prod does not.
--
-- Nullable with no default: catalog-only in Postgres, instant at any table size.
--
-- Guarded on the TABLE as well as the column. `ADD COLUMN IF NOT EXISTS` only
-- protects against the column already existing — it still fails if the table is
-- absent, which is the case on a database that has not had the baseline applied.
SET lock_timeout = '3s';
SET statement_timeout = '30s';

DO $$
BEGIN
  IF to_regclass('public.prices') IS NULL THEN
    RAISE NOTICE 'prices does not exist here; nothing to do';
    RETURN;
  END IF;
  ALTER TABLE public.prices ADD COLUMN IF NOT EXISTS bucket_size character varying(20) NULL;
END $$;

-- migrate:down
SET lock_timeout = '3s';
DO $$
BEGIN
  IF to_regclass('public.prices') IS NOT NULL THEN
    ALTER TABLE public.prices DROP COLUMN IF EXISTS bucket_size;
  END IF;
END $$;
