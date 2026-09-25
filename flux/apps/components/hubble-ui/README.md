# Hubble UI standalone

Standalone Hubble UI (service map) backed by the Hubble Relay — the Relay
ships in the cilium component, NOT here.

## Layout

`base/` holds every manifest (`hubble-ui.yaml` + `oauth2-proxy.yaml`
OCIRepository + HelmRelease, secrets, wildcard certificate, HTTPRoute); env
overlays `{dev,prd}/` patch hostnames, vault refs, and proxy values via
`resources: [../base]`. Tenant is `apps/hubble-ui` via
`flux/apps/update-policies/hubble-ui.yaml`.

## Relay backend (by DNS name)

The `backend` container points at the Relay Service by DNS name
(`FLOWS_API_ADDR=hubble-relay.cilium.svc:80` — port 80 = relay server TLS OFF).
Images pin to the Cilium v1.20.2 chart defaults: frontend
`quay.io/cilium/hubble-ui:v0.13.6`, backend
`quay.io/cilium/hubble-ui-backend:v0.13.6` (move together on every Cilium
minor; Changelog: https://github.com/cilium/cilium/releases — the UI ships
with Cilium). Nginx front door (`hubble-ui-nginx` ConfigMap) renders from the
chart's `hubble-ui/_nginx.tpl` (baseUrl `/`). RBAC: the apps-tenant Flux SA
cannot manage cluster-scoped RBAC, so this grants a namespaced Role
(pods/services/endpoints read in-namespace) instead of the chart's ClusterRole.

## Auth

Per-instance `oauth2-proxy` (official OCI chart
`oci://ghcr.io/oauth2-proxy/charts/oauth2-proxy:10.7.0`, app `v7.15.4`)
fronts the UI; the HTTPRoute backends to the proxy (`:4180`), which
upstreams to `http://hubble-ui.hubble-ui.svc:80`:

| Item | Value |
|---|---|
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Client | `hubble` (secret via ESO, never Git) |
| Redirect | `https://hubble.home-ops.yansyah.my.id/oauth2/callback` (covered by `https://*/oauth2/callback`) |
| Cookie domain | `.home-ops.yansyah.my.id` (secure, samesite=lax) |
| Scopes | `openid profile email groups` |
| Gate | `allowed-group=hubble-ui-admin` |
| Flags | `reverse-proxy=true`, `skip-provider-button=true` |

Secrets: `ExternalSecret/oauth2-proxy` syncs `client-id` + `client-secret` +
`cookie-secret` from `hubble-ui-sso-outputs` via the in-cluster
`hubble-ui-k8s` SecretStore (stored outputs — no pass:// seeding). The
`hubble` client is owned by this app's `hubble-ui-sso` Terraform CR (no
`org_id` literal in git).

## Routing

`base/hubble-httproute.yaml` — HTTPRoute on the shared `main` Gateway
(cross-namespace parentRef, `https` section): hostname
`hubble.home-ops.yansyah.my.id`, `/` → `oauth2-proxy:4180`. TLS terminates
at the Gateway via the in-namespace wildcard `Certificate`
(cert-manager Secrets are namespace-local).

## Telemetry / monitoring / updates

- No usage-reporting keys; no analytics env/args on either container.
- No `ServiceMonitor`.
- Updates via `flux/apps/update-policies/hubble-ui.yaml` (`$imagepolicy`
  markers on all three images; UI floor `>=0.13.6`). The oauth2-proxy
  markers are shared with clickstack + flux-operator-ui — bump together.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | hubble-ui 1, oauth2-proxy 1 | hostnames, vault refs, proxy values |
| `prd` | hubble-ui 2, oauth2-proxy 1 (singleton in every env) | hostnames, vault refs, proxy values + hubble-ui `replicas` → 2 |

oauth2-proxy stays a singleton (1) in every env — never scale it.

Upstream reference (read-only): `/tmp/home-ops-docs/cilium-docs`.
