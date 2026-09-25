# otel-collectors

Operator-managed `OpenTelemetryCollector` CRs (contrib-collector 0.159.0,
markers `infra:otel-collector:tag`): `otel-agent` DaemonSet (kubeletstats +
filelog -> gateway OTLP) and `otel-gateway` StatefulSet (OTLP + k8s_cluster +
TargetAllocator-scraped Prometheus -> ClickHouse).

TargetAllocator runs on the gateway only (consistent-hashing,
`prometheusCR.enabled: true`, `otel-scrape: "true"` matchLabels selectors over
monitors + namespaces). Scoped `otel-collector` / `otel-targetallocator`
ClusterRoles in `controllers/base/rbac.yaml` (operator creates the
ServiceAccounts from each CR).

Gateway exports to the shared infra CHI (`clickhouse-clickhouse.clickhouse`,
`create_schema: true`, exporter-internal `sending_queue.batch` 5000/10s,
`ttl: 0s` — DBA-managed table TTLs default 30d). Auth via
`ExternalSecret/otel-clickhouse` (proton-pass `pass://<cluster>/otel-collectors/clickhouse-password`).
ClusterIP only — no Ingress/Gateway (OTLP stays in-cluster).

## Environments

| Env | Gateway replicas | Database |
| --- | --- | --- |
| `dev` | 1 | `otel_dev` |
| `prd` | 2 (base values) | `otel_prd` |

## Telemetry / monitoring / updates

No usage reporting. Bumps: `update-policies/otel-collectors.yaml`
(contrib >=0.159.0, markers `infra:otel-collector:tag`) -> PR automation.
Changelogs: https://github.com/open-telemetry/opentelemetry-collector-releases/releases,
https://github.com/open-telemetry/opentelemetry-collector-contrib/releases.
