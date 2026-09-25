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
  v1.46.0), exporting OTLP/HTTP to the OTLP endpoint below. View in HyperDX.

## OTLP endpoint convention

- **Infra services** (`apprise-go-api`, `eso-proton-pass`, `external-dns`)
  export to the infra pipeline at
  `http://otel-gateway-collector.otel-collectors.svc:4318` via an explicit
  `OTEL_EXPORTER_OTLP_ENDPOINT` env in each manifest — never to an apps
  collector (`infra/components/<name>/{controllers,configs}/base/*.yaml`).
- **Apps-namespace workloads** (clickstack app) export to the in-namespace
  collector (`apps/components/clickstack/base/clickstack.yaml`).
- The Go code default is the gateway value; `OTEL_SDK_DISABLED=true` disables
  export for tests and local runs.

## When a chart ships a ServiceMonitor

New charts render their monitors unconditionally and rely on the
`infra-crds` gate — monitor-producing infra tenants gate
`infra-controllers` on the otel-operator `infra-crds` Established
healthChecks (`flux/fleet/tenants/infra.yaml`), so monitors never render
before the CRDs exist. Charts whose template errors without the CRDs carry a
chart-native guard instead (ESO `renderMode: skipIfMissing`,
`infra/components/external-secrets/controllers/base/externalsecrets.yaml`)
with a why-comment. The sanctioned on-state is `enabled: true` plus the
`otel-scrape: "true"` label, applied from dev/prd overlays only. Never add
Prometheus/Grafana to consume the monitors.

## Go instrumentation recipe

Every Go service (`projects/`) follows the `apprise-go-api` shape:
`tracing.Setup` in `main`, `tracing.Middleware` on the mux (`/metrics` and
`/healthz` bypass observation), `/metrics` + `/healthz` + `/readyz` on every
service (probes target `/healthz` + `/readyz`), `OTEL_EXPORTER_OTLP_ENDPOINT`
(defaults to the gateway endpoint above) with `OTEL_SDK_DISABLED=true` for
dependency-free tests and local runs.

## Always-on ordering

- **Infra:** otel-collectors `infra-controllers` gates on the otel-operator
  controllers + the shared infra CHI configs; monitor-producing tenants gate
  on the otel-operator `infra-crds` Established healthChecks
  (`flux/fleet/tenants/infra.yaml`). The gateway `__OTEL_DATABASE__` shape
  substitutes in the otel-collectors controllers overlays at the real field
  `/spec/config/exporters/clickhouse/database`; the clickhouse CHI's S3
  prereqs (bucketclaims/cosi-keys/s3-credentials) live in its controllers
  base so the CHI never races its bucket/keys.
- **Apps:** the apps ResourceSet gates on the cnpg/clickhouse/cosi/
  otel-operator/otel-collectors `infra-configs` Ready
  (`flux/fleet/tenants/apps.yaml`) — storage/observability consumers never
  reconcile before their backends. New apps depending on storage/observability
  stay covered by these gates.
- **Clickstack:** backends first in `base/kustomization.yaml` build order
  (RBAC/buckets/secrets → CHI/CNPG → proxy → app/collector → front/HPA/VPA →
  Terraform).

## Discovery (TargetAllocator)

Every tenant namespace (infra + apps) carries `otel-scrape: "true"` from the
fleet ResourceSet Namespace templates (`flux/fleet/tenants/infra.yaml`,
`flux/fleet/tenants/apps.yaml`); the gateway TargetAllocator scrapes only
monitors + namespaces carrying that label (object and namespace selectors
must both match). Monitor-object labels land via each chart's label knob
from the dev/prd overlay patches.

## Probes

`/healthz` liveness + `/readyz` readiness on every Deployment, values tuned per
component docs. Slow-starting dependencies gate behind a `startupProbe` instead
of the chart default (Zitadel server 60s, login 30s) so CNPG migrations and OIDC
warmup never trip restarts. Upstream images with fixed paths keep them with a
why-comment (HyperDX app `/health`, collector `/`, FerretDB TCP —
`apps/components/clickstack/base/{clickstack,ferretdb}.yaml`).

## VPA

VPA `Off` alongside any HPA; never combine an active VPA mode with HPA on the
same workload. Every singleton with no HPA uses `updateMode: Initial` —
recommendations apply at pod (re)start only, never mid-run eviction.

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
