# Hubble UI standalone

Standalone Hubble UI (service map) backed by the Hubble Relay — the Relay ships in the cilium component, not here.

## Layout

`base/` holds every manifest (`hubble-ui.yaml`, `oauth2-proxy.yaml`, secrets, wildcard certificate, HTTPRoute); `{dev,prd}/` patch hostnames, vault refs, and proxy values.

## Relay backend

`backend` points at the Relay by DNS name (`FLOWS_API_ADDR=hubble-relay.cilium.svc:80`). Images pin to the Cilium v1.20.2 chart defaults: frontend `quay.io/cilium/hubble-ui:v0.13.6`, backend `quay.io/cilium/hubble-ui-backend:v0.13.6` (move together on every Cilium minor). Namespaced Role (not the chart ClusterRole) — the apps-tenant SA cannot manage cluster-scoped RBAC.

## Auth

Per-instance `oauth2-proxy` (chart `10.7.0`, app `v7.15.4`) fronts the UI (`:4180` → `http://hubble-ui.hubble-ui.svc:80`):

| Item | Value |
|---|---|
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Client | `hubble` |
| Redirect | `https://hubble.home-ops.yansyah.my.id/oauth2/callback` |
| Cookie | `_oauth2_proxy_hubble_ui`, domain `.home-ops.yansyah.my.id` |
| Gate | `allowed-group=hubble-ui-admin` |

## Credentials

`oauth2-proxy` ExternalSecret (client-id + client-secret + cookie-secret) from `hubble-ui-sso-outputs` (Terraform-owned via `hubble-ui-k8s` store, no pass:// seeding).

## Routing

`hubble.home-ops.yansyah.my.id`, `/` → `oauth2-proxy:4180` on shared `main` Gateway. TLS via the in-namespace wildcard Certificate.

## Probes

Fixed-path exceptions (why-comments in `base/hubble-ui.yaml`): frontend `/healthz` + `/` on `:8081`; backend TCP sockets on `:8090` (gRPC only).

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | hubble-ui 1, oauth2-proxy 1 | hostnames, vault refs, proxy values |
| `prd` | hubble-ui 2, oauth2-proxy 1 (singleton in every env) | hostnames, vault refs, proxy values |

## Updates

Policy `update-policies/hubble-ui.yaml` (`$imagepolicy` markers on all three images; UI floor `>=0.13.6`; proxy markers shared with clickstack + flux-operator-ui, bump together). Changelog: [cilium](https://github.com/cilium/cilium/releases).
