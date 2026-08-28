# Deploy wiring: allow-all Kafka ACL seed

## Status: WIRED — folded into `migrate kafka`

There is no separate `kafka-acls` subcommand anymore. `migrate kafka`
(topic reconcile) now also seeds the allow-all ACL safety net, reusing the
same cluster admin connection it already opens for topic reconciliation.
The ACL seed is gated internally to SCRAM (MSK) — a no-op on OAUTHBEARER
(GCP Managed Kafka) or any other mechanism — and honors `--dry-run`.

In the Helm chart
(`internal/ee/infrastructure/helm/flexprice/templates/jobs/migration.yaml`),
the `run-kafka-migrate` initContainer runs `${MIGRATE_BINARY} kafka`, guarded
by `{{- if and .Values.migration.steps.kafka (and .Values.kafkaConfig.useSASL (ne .Values.kafkaConfig.saslMechanism "OAUTHBEARER")) }}`
(same SCRAM/MSK gate as before). The pre-existing `create-kafka-topics` step
(CLI sidecar, `kafka-topics.sh`) is unchanged — the Go step's topic reconcile
is a redundant, idempotent safety pass on top of it.

For any non-chart deploy path (e.g. a BYOC script running migrations without
this Helm chart), call `./migrate kafka` — no separate ACL call needed.

## Config / secrets

No new env vars or secrets required. Reuses the same Kafka connection config
as before: `FLEXPRICE_KAFKA_*` (brokers, SASL/TLS settings) via the standard
Viper-loaded `config.yaml`/env stack.
