# otel-collectors

Operator-managed `OpenTelemetryCollector` CRs (contrib-collector 0.159.0, markers `infra:otel-collector:tag`): `otel-agent` DaemonSet (kubeletstats + filelog -> gateway OTLP) and `otel-gateway` StatefulSet (OTLP + k8s_cluster + TargetAllocator-scraped Prometheus -> ClickHouse). TargetAllocator runs on the gateway only (`prometheusCR.enabled: true`, `otel-scrape: "true"` selectors over monitors + namespaces). ClusterIP only — OTLP stays in-cluster.

Gateway exports to the shared infra CHI (`clickhouse-clickhouse.clickhouse`, `create_schema: true`, `ttl: 0s` — DBA-managed table TTLs default 30d). Auth as the dedicated `otel` CHI user via `ExternalSecret/otel-clickhouse` (`pass://<cluster>/otel-collectors/clickhouse-password` — same value as `pass://<cluster>/clickhouse/otel-password`; one password, two vault mirrors); never the `default` user.

## Environments

| Env | Gateway replicas | Database |
| --- | --- | --- |
| `dev` | 1 | `otel_dev` |
| `prd` | 2 (base values) | `otel_prd` |

## Updates

`update-policies/otel-collectors.yaml` (contrib >=0.159.0, markers `infra:otel-collector:tag`). Changelogs: https://github.com/open-telemetry/opentelemetry-collector-releases/releases, https://github.com/open-telemetry/opentelemetry-collector-contrib/releases.
