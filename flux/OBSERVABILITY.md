# Observability (ClickHouse-native)

ClickHouse is the only telemetry store; HyperDX is the only telemetry UI. There is no Prometheus or
Grafana in any cluster. Logs, metrics, and traces flow through the OTel pipeline
(`infra/components/otel-operator`, `infra/components/otel-collectors`) into namespace-local
ClickHouse and are read back in HyperDX. See `apps/components/clickstack/README.md` for the
ClickStack deployment itself.

## OTel pipeline (landed)

- **Operator** (`infra/components/otel-operator/`): `crds/base` vendors ServiceMonitor + PodMonitor
  CRDs (prometheus-operator v0.93.1, monitoring scope only) and renders through the fleet's
  `prune:false` infra-crds Kustomization (`tenants/infra.yaml`, like gateway-api + cosi);
  `controllers/{base,dev,prd}` run the operator chart (0.123.1/app 0.159.0) with
  `infra:otel-operator:tag` update policy (`update-policies/otel-operator.yaml`).
- **Collectors** (`infra/components/otel-collectors/`): `controllers/{base,dev,prd}` hold operator-managed
  `OpenTelemetryCollector` CRs — `otel-agent` DaemonSet (kubeletstats + filelog → gateway OTLP) and
  `otel-gateway` StatefulSet (dev 1 / prd 2, `otel_dev` / `otel_prd`) with
  `infra:otel-collector:tag` update policy (`update-policies/otel-collectors.yaml`).
- **Discovery:** the gateway's TargetAllocator (consistent-hashing, `prometheusCR.enabled`) scrapes only
  monitors + namespaces labeled `otel-scrape: "true"` (object and namespace selectors must both
  match). Tenant namespaces carry the label from the fleet `tenants/infra.yaml` Namespace template;
  each flipped monitor carries it via its chart label knob — ESO via `serviceMonitor.additionalLabels`
  from the dev/prd overlay patches. The agent uses static targets.
- **ClickHouse export:** gateway `clickhouse` exporter points at the shared infra CHI
  (`tcp://clickhouse-clickhouse.clickhouse.svc:9000`, `create_schema: true`), `ttl: 0s` (DBA-managed
  table TTLs, default 30d) and exporter-internal `sending_queue.batch` 5000/10s; no custom TTLs in git
  (see ClickHouse retention below).
- **Auth:** `configs/base/clickhouse-credentials.yaml` `ExternalSecret/otel-clickhouse` reads
  `pass://<cluster>/otel-collectors/clickhouse-password` (`default` user, empty password against the
  shared infra CHI). Seed per env before first install:

  ```shell
  pass-cli item create 'acme-dev-bdo1-talos-apps-01/otel-collectors/clickhouse-password'
  pass-cli item create 'acme-prd-bdo1-talos-apps-01/otel-collectors/clickhouse-password'
  ```

  Secret gate not run from this workspace (`pass-cli info` is agent-blocked); run it by hand before
  seeding — it must succeed (logged in) or ESO pending Secrets are expected.

## Per-signal guide

- **Logs:** Go services emit JSON `slog` with `trace_id` / `span_id` correlation fields (see Go recipe).
  Log output is collected by the in-namespace OTel collector and stored in ClickHouse; read it in HyperDX.
- **Metrics:** every Go service keeps a Prometheus-exposition `/metrics` endpoint. `ServiceMonitor` /
  `PodMonitor` objects exist only as scrape-target discovery for the OTel pipeline — never as a reason
  to deploy Prometheus. See the 4-step chart rule below.
- **Traces:** Go services use the OTel trace SDK (`internal/tracing`, SDK v1.46.0), exporting OTLP/HTTP
  to the in-namespace collector at `http://clickstack-otel-collector.clickstack.svc:4318`. View traces
  in HyperDX.

## When a chart ships a ServiceMonitor (4 steps)

1. Keep it off by default (`enabled: false` or the chart's equivalent) with a keep-off comment until
   its owner flips it.
2. The sanctioned on-state is `enabled: true` plus the `otel-scrape: "true"` label so the gateway TA
   scrapes it into ClickHouse — the namespace label comes from the fleet `tenants/infra.yaml`
   template; the monitor-object label comes via the chart's label knob (ESO:
   `serviceMonitor.additionalLabels`, applied from the dev/prd overlay patches, never base-values
   edits).
3. Guarded charts use a chart-native guard as the template — ESO's `renderMode: skipIfMissing`
   (`infra/components/external-secrets/controllers/base/externalsecrets.yaml` `serviceMonitor` block)
   renders only where monitor CRDs exist. `tofu-controller` and Zitadel flip in two steps
   (`metrics.enabled: true` first so the endpoint exists, then the monitor); their comments say so.
4. Never add Prometheus/Grafana to consume the monitors. Discovery feeds the OTel → ClickHouse
   pipeline; HyperDX is the UI.

## Go instrumentation recipe

Every Go service (`projects/`) follows the `apprise-go-api` shape:

1. Call `tracing.Setup(ctx, serviceName, serviceVersion)` in `main`; wire the returned shutdown func.
2. Wrap the mux: `tracing.Middleware("<service>", "/metrics", "/healthz")` — `/metrics` and `/healthz`
   bypass observation entirely (no counter inc, no spans).
3. Keep `/metrics` (Prometheus exposition), `/healthz` (static `{"status":"ok"}`, zero downstream
   calls), and `/readyz` on every service; probes target `/healthz` + `/readyz`.
4. Env vars: `OTEL_EXPORTER_OTLP_ENDPOINT` (defaults to the in-namespace collector `:4318`),
   `OTEL_SERVICE_NAME` (wins over the code default), `OTEL_SDK_DISABLED=true` (leaves globals
   untouched — tests and local runs stay dependency-free).

## Probes

`/healthz` liveness + `/readyz` readiness on every Deployment, values tuned per component docs.
Slow-starting dependencies gate liveness/readiness behind a `startupProbe` instead of the chart
default (Zitadel server: 60s budget; login: 30s budget) so CNPG migrations and OIDC warmup never
trip restarts.

## VPA

VPA `Off` alongside any HPA; never combine an active VPA mode with HPA on the same workload.
Singletons with no HPA by design (`external-dns`, `tofu-controller`) use `updateMode: Initial` —
recommendations apply at pod (re)start only, never evicting the running singleton mid-sync.

## ClickHouse retention

No custom TTLs are configured in git; table retention follows the ClickHouse/HyperDX chart defaults
and HyperDX manages its own tables. Adding explicit TTLs is a future change, not current state.

## HyperDX route / Gateway pattern

HyperDX declares its own `HTTPRoute` on the shared `main` Gateway (cross-namespace `parentRefs`,
`https` section): `clickstack.<domain>` → `oauth2-proxy:4180`, which upstreams to
`http://clickstack.clickstack.svc:3000`. TLS terminates at the Gateway via the in-namespace wildcard
`Certificate`. Access is gated by Zitadel OIDC (`allowed-group=clickstack-admin`).

## Alert path

HyperDX webhook → `apprise-go-api` `/notify` → Matrix room. `apprise-go-api` is the single delivery
endpoint for notifications; alerting never bypasses it.

## Upgrades

Changelog-first plus pin parity (see AGENTS.md Reference/Patterns): ClickHouse server moves in
26.8 LTS lockstep across the infra operator line and the app CHI marker
(`infra:clickhouse-server:tag`); the OTel SDK bumps across all three Go services together; the
`oauth2-proxy` chart/app markers move together with hubble-ui + flux-operator-ui.
