# Observability (ClickHouse-native)

ClickHouse is the only telemetry store; HyperDX is the only telemetry UI. No Prometheus or Grafana. Logs, metrics, and traces flow through the OTel pipeline (`infra/components/otel-operator`, `infra/components/otel-collectors`) into namespace-local ClickHouse and are read back in HyperDX. See `apps/components/clickstack/README.md` for the ClickStack deployment.

## OTel pipeline

- **Operator** (`infra/components/otel-operator/`): ServiceMonitor + PodMonitor CRDs via first-party `helm-otel-monitoring-crds` (CreateReplace), rendered through the fleet `prune:false` infra-crds Kustomization; controllers run the operator chart (0.123.1/app 0.159.0).
- **Collectors** (`infra/components/otel-collectors/`): operator-managed `OpenTelemetryCollector` CRs — `otel-agent` DaemonSet (kubeletstats + filelog → gateway OTLP) and `otel-gateway` StatefulSet (dev 1 / prd 2).
- **ClickHouse export:** gateway `clickhouse` exporter points at the shared infra CHI (`tcp://clickhouse-clickhouse.clickhouse.svc:9000`, `create_schema: true`).
- **Auth:** `ExternalSecret/otel-clickhouse` reads `pass://<cluster>/otel-collectors/clickhouse-password`; workloads pend until ESO syncs.

## OTLP endpoint convention

- **Infra services** export to `http://otel-gateway-collector.otel-collectors.svc:4318` via explicit `OTEL_EXPORTER_OTLP_ENDPOINT` env — never to an apps collector.
- **Apps-namespace workloads** (clickstack app) export to the in-namespace collector.
- `OTEL_SDK_DISABLED=true` disables export for tests and local runs.

## Per-signal guide

- **Logs:** Go services emit JSON `slog` with `trace_id` / `span_id`; collected by the in-namespace collector, stored in ClickHouse, read in HyperDX.
- **Metrics:** every Go service keeps a Prometheus-exposition `/metrics` endpoint. `ServiceMonitor` / `PodMonitor` objects are scrape-target discovery for the OTel pipeline only — never a reason to deploy Prometheus.
- **Traces:** Go services use the OTel trace SDK (`internal/tracing`, SDK v1.46.0), exporting OTLP/HTTP. View in HyperDX.

## Go instrumentation recipe

Every Go service follows the `apprise-go-api` shape: `tracing.Setup` in `main`, `tracing.Middleware` on the mux (`/metrics` + `/healthz` bypass), `/metrics` + `/healthz` + `/readyz` on every service (probes target `/healthz` + `/readyz`).

## When a chart ships a ServiceMonitor

Monitors render unconditionally behind the `infra-crds` gate (monitor-producing tenants gate on otel-operator `infra-crds` Established healthChecks); charts whose template errors without the CRDs carry a chart-native guard with a why-comment. Sanctioned on-state: `enabled: true` + label `otel-scrape: "true"`, applied from dev/prd overlays only.

## Ordering gates

- **Infra:** otel-collectors gates on the otel-operator controllers + shared infra CHI; monitor-producing tenants gate on otel-operator `infra-crds` Established (`flux/fleet/tenants/infra.yaml`).
- **Apps:** apps ResourceSet gates on cnpg/clickhouse/cosi/otel-operator/otel-collectors `infra-configs` Ready (`flux/fleet/tenants/apps.yaml`).
- **Clickstack:** backends first in `base/kustomization.yaml` build order (RBAC/buckets/secrets → CHI/CNPG → proxy → app/collector → front/HPA/VPA → Terraform).

## Discovery (TargetAllocator)

Every tenant namespace carries `otel-scrape: "true"` from the fleet ResourceSet Namespace templates; the gateway TargetAllocator scrapes only monitors + namespaces carrying that label (both selectors must match). Monitor-object labels land via each chart's label knob from overlay patches.

## Retention

- **Signal tables:** `HYPERDX_OTEL_EXPORTER_TABLES_TTL` default `720h` (30 days) across logs/traces/metrics/sessions (collector reconciles TTLs on existing tables).
- **System tables** (`query_log`, `part_log`, `text_log`, `metric_log`, `asynchronous_metric_log`): `event_date + INTERVAL 7 DAY DELETE` via the chart's `extraConfig`, plus `logger` capped at `information` / `100M` x 10 files.
- Follow-up: raise per-signal TTLs (e.g. 180d) once prd disk headroom is measured.

## HyperDX route

HyperDX declares its own `HTTPRoute` on shared `main` Gateway: `clickstack.<domain>` → `oauth2-proxy:4180` → `http://clickstack.clickstack.svc:3000`. TLS terminates at the Gateway via the in-namespace wildcard Certificate. Gated by Zitadel OIDC (`allowed-group=clickstack-admin`).

## Alert path

HyperDX webhook → `apprise-go-api` `/notify` → Matrix room. `apprise-go-api` is the single delivery endpoint.
