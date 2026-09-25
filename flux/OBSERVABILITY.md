# Observability (ClickHouse-native)

ClickHouse is the only telemetry store; HyperDX is the only telemetry UI. There is no Prometheus or
Grafana in any cluster. Logs, metrics, and traces flow through OTel collectors into namespace-local
ClickHouse and are read back in HyperDX. See `apps/components/clickstack/README.md` for the
ClickStack deployment itself.

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

1. Keep it off by default (`enabled: false` or the chart's equivalent) with a keep-off comment.
2. The only sanctioned on-state is a chart-native guard such as ESO's
   `renderMode: skipIfMissing` (`infra/components/external-secrets/controllers/base/externalsecrets.yaml`
   `serviceMonitor` block) — it renders only where monitor CRDs exist.
3. Any flip from keep-off to guarded-on carries a comment stating why (which consumer needs discovery).
   `tofu-controller` and Zitadel stay keep-off; their comments say so.
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
