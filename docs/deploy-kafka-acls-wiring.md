# Deploy wiring: `migrate kafka-acls`

## Finding

Exhaustively grepped this worktree (Helm chart, Makefile, config, all
`.github/workflows/*.yml`, BYOC scripts in the infra repo) for any deploy-time
invocation of the Go `migrate kafka` subcommand (topic reconcile, aka
`kafka-migrate`, documented in `docs/KAFKA_MIGRATE.md`).

**Result: no deploy artifact invokes it.**

- `internal/ee/infrastructure/helm/flexprice/templates/jobs/migration.yaml` IS
  in-tree (not on a separate branch) and does run the `migrate` binary for
  `clickhouse` and `postgres` subcommands (`${MIGRATE_BINARY} clickhouse ...`,
  `${MIGRATE_BINARY} postgres ...`). Its `create-kafka-topics` step (guarded by
  `.Values.migration.steps.kafka`) does **not** call `migrate kafka` — it
  shells out directly to `kafka-topics`/`kafka-topics.sh` inside a
  `bitnami/kafka` or `cp-kafka` sidecar image instead.
- No `.github/workflows/*.yml` references `migrate kafka`.
- No BYOC script under `infrastructure/scripts/byoc` or `infrastructure/_stacks`
  references it either.
- `docs/KAFKA_MIGRATE.md` confirms this: the Go topic-reconcile binary is
  documented as a standalone tool that must be run manually — it was never
  wired into the deploy path.

So there is no existing "run `migrate kafka` here" step to mirror. This doc
is the wiring instruction for whoever next touches the migration Job template
or a BYOC deploy script.

## What to add

```
./migrate kafka-acls
```

This seeds allow-all Kafka ACLs. It is a **safe no-op on non-SCRAM clusters**
(gated internally on `sasl.mechanism` == a SCRAM variant), so it can run
**unconditionally** — no per-env guard, no `.Values.migration.steps.*` flag
needed.

## Where to add it

Immediately after whichever step ends up running Kafka topic reconciliation
in the deploy path, once one exists. Concretely:

- **Helm migration Job**
  (`internal/ee/infrastructure/helm/flexprice/templates/jobs/migration.yaml`):
  add a step right after the `create-kafka-topics` step (guarded the same way,
  `{{- if .Values.migration.steps.kafka }}`), using the same
  `${MIGRATE_BINARY}` resolution pattern already used for the `clickhouse`/
  `postgres` steps (lines ~297-352):
  ```sh
  ${MIGRATE_BINARY} kafka-acls || exit 1
  ```
- **BYOC deploy path**: any BYOC script in `infrastructure/scripts/byoc` that
  runs migrations should call `./migrate kafka-acls` right after its
  `./migrate kafka` (or topic-creation) call, once such a call exists there.

## Config / secrets

No new env vars or secrets required. `migrate kafka-acls` reuses the exact
same Kafka connection config as `migrate kafka` / the app itself:
`FLEXPRICE_KAFKA_*` (brokers, SASL/TLS settings) via the standard
Viper-loaded `config.yaml`/env stack.
