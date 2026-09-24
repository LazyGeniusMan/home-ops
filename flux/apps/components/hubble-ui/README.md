# Hubble UI standalone (§11.3)

Standalone Hubble UI (service map) backed by the §8.2 Hubble Relay — the
Relay itself ships in the cilium component, NOT here (no duplication).

## Layout (environment-direct, apps area)

`base/` holds every manifest (`hubble-ui.yaml` + `oauth2-proxy.yaml`
OCIRepository + HelmRelease, plus secrets, wildcard certificate,
HTTPRoute); env overlays `{dev,prd}/` patch hostnames, vault refs, and
proxy values via
`resources: [../base]`. Tenant is `apps/hubble-ui` via
`flux/apps/update-policies/hubble-ui.yaml`.

## Relay backend (by DNS name)

The `backend` container points at the Relay Service by DNS name (chart
default shape, cross-namespace):

- `FLOWS_API_ADDR=hubble-relay.cilium.svc:80` — Service `hubble-relay` in
  the `cilium` namespace (see the §8.2 cilium component), port 80 = relay
  server TLS OFF (no relay TLS block set in the §8.2 values, so the
  plaintext `:80` branch applies).
- Images pinned to the Cilium v1.20.2 chart defaults: frontend
  `quay.io/cilium/hubble-ui:v0.13.6`, backend
  `quay.io/cilium/hubble-ui-backend:v0.13.6`.
- Nginx front door (`hubble-ui-nginx` ConfigMap) rendered from the chart's
  `hubble-ui/_nginx.tpl` with the default baseUrl `/` (`:80 → 8081`,
  `/api → 127.0.0.1:8090`).
- RBAC note: the chart ships a ClusterRole (namespaces/nodes/pods/services
  + CRDs) for cluster-wide listing; the apps-tenant Flux SA cannot manage
  cluster-scoped RBAC, so this component grants a namespaced Role
  (pods/services/endpoints read in-namespace) instead. Cluster-wide service
  mapping stays limited until a cluster-scoped grant lands out-of-band.

## Auth (locked proxy contract)

Per-instance `oauth2-proxy` (official OCI chart
`oci://ghcr.io/oauth2-proxy/charts/oauth2-proxy:10.7.0`, app `v7.15.4`)
fronts the UI; the HTTPRoute backend points at the proxy (`:4180`), which
upstreams to `http://hubble-ui.hubble-ui.svc:80`:

| Item | Value |
|---|---|
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Client | `hubble` (secret via ESO, never Git) |
| Redirect | `https://hubble.home-ops.yansyah.my.id/oauth2/callback` (covered by the registered wildcard `https://*/oauth2/callback`) |
| Cookie domain | `.home-ops.yansyah.my.id` (secure, samesite=lax) |
| Scopes | `openid profile email groups` (groups claim `groups`) |
| Gate | `allowed-group=admin` |
| Flags | `reverse-proxy=true`, `skip-provider-button=true` |

Secrets: `ExternalSecret/oauth2-proxy` syncs `client-id` + `client-secret`
(both generated server-side) + `cookie-secret` (32 random bytes, generated
in-Tofu) from the `hubble-ui-sso-outputs` Secret through the in-cluster
`hubble-ui-k8s` SecretStore (stored outputs, end-to-end — NO pass:// seeding
for OIDC creds). The Zitadel `hubble` client is owned by this app's
`hubble-ui-sso` Terraform CR (upstream identity — org_id + admin ID —
mirrors from the FirstInstance handoff via the ESO-synced
`hubble-ui-terraform-vars` Secret, no `org_id` literal in git, no email
lookups; provider auth mirrors from the chart-kept handoff the same way).
Zero-UI: no pass:// SSO dependency remains.

## Routing

`base/hubble-httproute.yaml` — HTTPRoute on the shared §8.1 Gateway
(`main`, cross-namespace parentRef, `https` section): hostname
`hubble.home-ops.yansyah.my.id`, `/` → `oauth2-proxy:4180`. TLS terminates
at the Gateway via the in-namespace wildcard `Certificate`
(`wildcard-certificate.yaml`, same duplicate pattern as §8.1 — cert-manager
Secrets are namespace-local).

## Telemetry-off / monitoring / updates

- Telemetry evidence: the Cilium v1.20.2 chart exposes no usage-reporting
  keys (checked at authoring: non-comment values lines matching
  `telemetry|usageReporting|phoneHome|analytics` are empty), so there is
  nothing to switch off. No analytics env/args are set on either container.
- Unguarded monitors OFF: no `ServiceMonitor` objects are shipped until
  `monitoring.coreos.com` CRDs land (same §9 deviation). Flip: add
  `ServiceMonitor`s for the relay metrics port (9966, §8.2 exposes it with
  `prometheus.enabled: true`) and the UI once the monitoring stack exists.
- Updates flow through `flux/apps/update-policies/hubble-ui.yaml`
  (ImageRepository + ImagePolicy, `$imagepolicy` markers on all three
  images; the UI floor `>=0.13.6` tracks the UI image line).

## Upgrade runbook

- Version source: frontend + backend image tags in
  `base/hubble-ui.yaml` (v0.13.6 — MUST match the Cilium v1.20.2 chart
  defaults; the Relay backend lives in the infra cilium component, NOT
  here).
- Changelog: https://github.com/cilium/cilium/releases (the UI ships
  with Cilium — there is no separate UI release line).
- Bump: on every Cilium minor, check the new chart's hubble-ui defaults
  first, then move frontend + backend together via the ImagePolicy PRs
  (markers `apps:hubble-ui:tag` + `apps:hubble-ui-backend:tag`,
  `update-policies/hubble-ui.yaml`). The oauth2-proxy markers
  (`apps:oauth2-proxy` + `apps:oauth2-proxy-chart`) are shared with
  clickstack + flux-operator-ui — bump all three apps' proxy pins
  together.
- Verify: the service map renders live flows via
  `hubble-relay.cilium.svc:80` and OIDC login still gates the UI.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | hubble-ui 1, oauth2-proxy 1 (single-instance) | hostnames, vault refs, proxy values + `replicas` → 1 each |
| `prd` | hubble-ui 2, oauth2-proxy 1 (recommended production) | hostnames, vault refs, proxy values + hubble-ui `replicas` → 2, oauth2-proxy stays 1 |

oauth2-proxy stays a singleton (1) in every env — never scale it.

Upstream reference (read-only): `/tmp/home-ops-docs/cilium-docs` (Relay ships in §8.2).
