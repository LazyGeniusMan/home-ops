# Hubble UI standalone (§11.3)

Standalone Hubble UI (service map) backed by the §8.2 Hubble Relay — the
Relay itself ships in the cilium component, NOT here (no duplication).

## Layout (environment-direct, apps area)

`base/` holds every manifest (`hubble-ui.yaml` + `oauth2-proxy.yaml`
workload, plus secrets, wildcard certificate, HTTPRoute); env overlays
`{dev,staging,production}/` patch hostnames, vault refs, and proxy args via
`resources: [../base]`. Tenant is `apps/hubble-ui` via
`flux/apps/update-policies/hubble-ui.yaml`.

## Relay backend (by DNS name)

The `backend` container points at the Relay Service by DNS name (chart
default shape, cross-namespace):

- `FLOWS_API_ADDR=hubble-relay.cilium.svc:80` — Service `hubble-relay` in
  the `cilium` namespace (see the §8.2 cilium component), port 80 = relay
  server TLS OFF (no relay TLS block set in the §8.2 values, so the
  plaintext `:80` branch applies).
- Images pinned to the Cilium v1.20.1 chart defaults: frontend
  `quay.io/cilium/hubble-ui:v0.13.5`, backend
  `quay.io/cilium/hubble-ui-backend:v0.13.5`.
- Nginx front door (`hubble-ui-nginx` ConfigMap) rendered from the chart's
  `hubble-ui/_nginx.tpl` with the default baseUrl `/` (`:80 → 8081`,
  `/api → 127.0.0.1:8090`).
- RBAC note: the chart ships a ClusterRole (namespaces/nodes/pods/services
  + CRDs) for cluster-wide listing; the apps-tenant Flux SA cannot manage
  cluster-scoped RBAC, so this component grants a namespaced Role
  (pods/services/endpoints read in-namespace) instead. Cluster-wide service
  mapping stays limited until a cluster-scoped grant lands out-of-band.

## Auth (locked proxy contract)

Per-instance `oauth2-proxy` (`quay.io/oauth2-proxy/oauth2-proxy:v7.6.0`)
fronts the UI; the HTTPRoute backend points at the proxy (`:4180`), which
upstreams to `http://hubble-ui.hubble-ui.svc:80`:

| Item | Value |
|---|---|
| Issuer | `https://zitadel.home-ops.yansyah.my.id` |
| Client | `oauth2-proxy-shared` (secret via ESO, never Git) |
| Redirect | `https://hubble.home-ops.yansyah.my.id/oauth2/callback` (covered by the registered wildcard `https://*/oauth2/callback`) |
| Cookie domain | `.home-ops.yansyah.my.id` (secure, samesite=lax) |
| Scopes | `openid profile email groups` (groups claim `groups`) |
| Gate | `allowed-group=admin` |
| Flags | `reverse-proxy=true`, `skip-provider-button=true` |

Secrets (`ExternalSecret/oauth2-proxy`): `client-secret` + `cookie-secret`
(32 random bytes) from Proton Pass
(`pass://acme-prd-bdo1-talos-apps-01/hubble-ui/oauth2-proxy-*`). Seed the
vault entries with pass-cli. The Zitadel `hubble` client is already declared
— reference only.

## Routing

`base/hubble-httproute.yaml` — HTTPRoute on the shared §8.1 Gateway
(`main`, cross-namespace parentRef, `https` section): hostname
`hubble.home-ops.yansyah.my.id`, `/` → `oauth2-proxy:4180`. TLS terminates
at the Gateway via the in-namespace wildcard `Certificate`
(`wildcard-certificate.yaml`, same duplicate pattern as §8.1 — cert-manager
Secrets are namespace-local).

## Telemetry-off / monitoring / updates

- Telemetry evidence: the Cilium v1.20.1 chart exposes no usage-reporting
  keys (checked at authoring: non-comment values lines matching
  `telemetry|usageReporting|phoneHome|analytics` are empty), so there is
  nothing to switch off. No analytics env/args are set on either container.
- Unguarded monitors OFF: no `ServiceMonitor` objects are shipped until
  `monitoring.coreos.com` CRDs land (same §9 deviation). Flip: add
  `ServiceMonitor`s for the relay metrics port (9966, §8.2 exposes it with
  `prometheus.enabled: true`) and the UI once the monitoring stack exists.
- Updates flow through `flux/apps/update-policies/hubble-ui.yaml`
  (ImageRepository + ImagePolicy, `$imagepolicy` markers on all three
  images; the UI floor `>=0.13.5` tracks the UI image line).

## Environments

`production` and `staging` currently inherit `../base` unchanged (same shape
as cert-manager before per-env divergence). Per-env tuning (replica count,
schedule knobs) lands with the first real divergence, not here.
