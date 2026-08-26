-- migrate:up
-- Narrow the entitlements uniqueness rule to exclude aggregation_mode='parallel'.
--
-- ent/schema/entitlement.go changed this predicate on 2026-07-25 (ae95c9d54).
-- AutoMigrate never applied it — ModifyIndex is in its skip set — so India prod
-- still enforces uniqueness across ALL published rows, which is broader than the
-- code intends.
--
-- Written to be safe on a database it has never seen, because the fleet is not
-- homogeneous: India prod, GCP staging (AlloyDB, DMS-migrated from AWS) and each
-- client hold different index sets under different names. Every step below checks
-- before acting and is a no-op where the condition does not hold.
--
-- Plain DDL rather than CONCURRENTLY: entitlements is ~365 rows / <1 MB, so the
-- ACCESS EXCLUSIVE lock lasts milliseconds. That trade would be wrong on events or
-- feature_usage — do not copy this shape onto a large table.
SET lock_timeout = '3s';

DO $$
DECLARE
  want text := '(((status)::text = ''published''::text) AND ((aggregation_mode)::text <> ''parallel''::text))';
  cols text := 'tenant_id, environment_id, entity_type, entity_id, feature_id';
  r    record;
BEGIN
  IF to_regclass('public.entitlements') IS NULL THEN
    RAISE NOTICE 'entitlements does not exist here; nothing to do';
    RETURN;
  END IF;

  -- Any UNIQUE index on exactly these columns whose predicate has not been
  -- narrowed yet. Matched on shape, not on name: the name is Ent-derived and
  -- differs between deployments.
  FOR r IN
    SELECT i.indexname, i.indexdef
    FROM pg_indexes i
    WHERE i.schemaname = 'public'
      AND i.tablename  = 'entitlements'
      AND i.indexdef LIKE 'CREATE UNIQUE INDEX%'
      AND i.indexdef LIKE '%(' || cols || ')%'
      AND i.indexdef NOT LIKE '%aggregation_mode%'
  LOOP
    RAISE NOTICE 'narrowing %', r.indexname;
    EXECUTE format('DROP INDEX public.%I', r.indexname);
    EXECUTE format(
      'CREATE UNIQUE INDEX %I ON public.entitlements (%s) WHERE %s',
      r.indexname, cols, want);
  END LOOP;
END $$;

-- migrate:down
-- Deliberately not reversible. Widening the predicate again would re-introduce a
-- uniqueness rule the application does not expect, and could reject writes that
-- are valid under the current code.
SELECT 'not reversible' AS note;
