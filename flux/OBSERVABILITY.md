# Observability (ClickHouse-native)

ClickHouse is the only telemetry store; HyperDX is the only telemetry UI. No
Prometheus or Grafana in any cluster. Logs, metrics, and traces flow through the
OTel pipeline (`infra/components/otel-operator`, `infra/components/otel-collectors`)
into namespace-local ClickHouse and are read back in HyperDX. See
`apps/components/clickstack/README.md` for the ClickStack deployment itself.

## OTel pipeline (landed)

- **Operator** (`infra/components/otel-operator/`): `crds/base` vendors
  ServiceMonitor + PodMonitor CRDs (prometheus-operator v0.93.1, monitoring
  scope only) rendered through the fleet's `prune:false` infra-crds
  Kustomization; `controllers/{base,dev,prd}` run the operator chart
  (0.123.1/app 0.159.0) with `infra:otel-operator:tag` update policy.
- **Collectors** (`infra/components/otel-collectors/`): operator-managed
  `OpenTelemetryCollector` CRs — `otel-agent` DaemonSet (kubeletstats + filelog
  → gateway OTLP) and `otel-gateway` StatefulSet (dev 1 / prd 2) with
  `infra:otel-collector:tag` update policy.
- **Discovery:** the gateway TargetAllocator scrapes only monitors + namespaces
  labeled `otel-scrape: "true"` (object and namespace selectors must both
  match). Tenant namespace labels come from the fleet `tenants/infra.yaml`
  template; monitor-object labels land via each chart's label knob from the
  dev/prd overlay patches (knob names in the overlay patch comments).
- **ClickHouse export:** gateway `clickhouse` exporter points at the shared
  infra CHI (`tcp://clickhouse-clickhouse.clickhouse.svc:9000`,
  `create_schema: true`); no custom TTLs in git (see retention below).
- **Auth:** `configs/base/clickhouse-credentials.yaml`
  `ExternalSecret/otel-clickhouse` reads
  `pass://<cluster>/otel-collectors/clickhouse-password`. Seed per env before
  first install; workloads pend until ESO syncs.

## Per-signal guide

- **Logs:** Go services emit JSON `slog` with `trace_id` / `span_id`
  correlation; collected by the in-namespace collector, stored in ClickHouse,
  read in HyperDX.
- **Metrics:** every Go service keeps a Prometheus-exposition `/metrics`
  endpoint. `ServiceMonitor` / `PodMonitor` objects are scrape-target discovery
  for the OTel pipeline only — never a reason to deploy Prometheus.
- **Traces:** Go services use the OTel trace SDK (`internal/tracing`, SDK
  v1.46.0), exporting OTLP/HTTP to the in-namespace collector at
  `http://clickstack-otel-collector.clickstack.svc:4318`. View in HyperDX.

## When a chart ships a ServiceMonitor

Keep off by default until its owner flips it. The sanctioned on-state is
`enabled: true` plus the `otel-scrape: "true"` label, applied from dev/prd
overlays only. Guarded charts use a chart-native guard as the template (ESO
`renderMode: skipIfMissing`); never add Prometheus/Grafana to consume the
monitors.

## Go instrumentation recipe

Every Go service (`projects/`) follows the `apprise-go-api` shape:
`tracing.Setup` in `main`, `tracing.Middleware` on the mux (`/metrics` and
`/healthz` bypass observation), `/metrics` + `/healthz` + `/readyz` on every
service (probes target `/healthz` + `/readyz`), `OTEL_EXPORTER_OTLP_ENDPOINT`
(defaults to the in-namespace collector `:4318`) with `OTEL_SDK_DISABLED=true`
for dependency-free tests and local runs.

## Probes

`/healthz` liveness + `/readyz` readiness on every Deployment, values tuned per
component docs. Slow-starting dependencies gate behind a `startupProbe` instead
of the chart default (Zitadel server 60s, login 30s) so CNPG migrations and OIDC
warmup never trip restarts.

## VPA

VPA `Off` alongside any HPA; never combine an active VPA mode with HPA on the
same workload. Singletons with no HPA by design (`external-dns`,
`tofu-controller`) use `updateMode: Initial` — recommendations apply at pod
(re)start only.

## ClickHouse retention

No custom TTLs in git; table retention follows the ClickHouse/HyperDX chart
defaults and HyperDX manages its own tables. Explicit TTLs are a future change,
not current state.

## HyperDX route / Gateway pattern

HyperDX declares its own `HTTPRoute` on the shared `main` Gateway:
`clickstack.<domain>` → `oauth2-proxy:4180`, which upstreams to
`http://clickstack.clickstack.svc:3000`. TLS terminates at the Gateway via the
in-namespace wildcard `Certificate`. Access is gated by Zitadel OIDC
(`allowed-group=clickstack-admin`).

## Alert path

HyperDX webhook → `apprise-go-api` `/notify` → Matrix room. `apprise-go-api` is
the single delivery endpoint; alerting never bypasses it.

## Upgrades

Changelog-first plus pin parity (see AGENTS.md Reference/Patterns): ClickHouse
server moves in 26.8 LTS lockstep across the infra operator line and the app CHI
marker (`infra:clickhouse-server:tag`); the OTel SDK bumps across all three Go
services together; the `oauth2-proxy` chart/app markers move together with
hubble-ui + flux-operator-ui.
