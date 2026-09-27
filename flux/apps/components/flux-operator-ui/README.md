# Flux Operator UI

Standalone Flux Web UI (`serverOnly`) in the apps tenant, auth-fronted by per-instance oauth2-proxy. UI only — bootstrap, fleet sync, and Managed resources are never touched here.

## Layout

`base/` holds every manifest (`flux-operator-ui.yaml`, `oauth2-proxy.yaml`, proxy credentials, wildcard certificate, HTTPRoutes); `{dev,prd}/` patch hostnames, vault refs, and proxy values.

## Pins

UI chart `oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator` tag **0.60.0**, in lockstep with `operator_chart_version` in `flux/fleet/terraform/versions.yaml`. Release `flux-operator-ui` with `web.serverOnly: true`, `installCRDs: false` (bootstrap owns CRDs and sync).

## RBAC

UI backend impersonates the authenticated user; grant users a read-only ClusterRole on the Flux CRD groups out-of-band — Flux never manages user RBAC here.

## Auth

| Item | Value |
| --- | --- |
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Client | `flux-operator-ui` (owned by `flux-operator-ui-sso` Terraform CR) |
| Scopes | `openid profile email groups` (`flux-operator-ui-admin` only) |
| Upstream | `http://flux-operator-ui.<ns>.svc:9080` |
| Callback | `https://flux-operator.home-ops.yansyah.my.id/oauth2/callback` |
| Cookie name | `_oauth2_proxy_flux_operator_ui` |

## Credentials

`oauth2-proxy-oidc` (client-id + client-secret from `flux-operator-ui-sso-outputs` via `flux-operator-ui-k8s` store); `oauth2-proxy-cookie` from `pass://acme-<env>-bdo1-talos-apps-01/flux-operator-ui/oauth2-proxy-cookie-secret`.

## Routing

`flux-operator.home-ops.yansyah.my.id` on shared `main` Gateway → `oauth2-proxy:4180`, never the UI directly. TLS via the in-namespace wildcard Certificate.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | UI 1, oauth2-proxy 1 | hostnames, vault refs, proxy args |
| `prd` | UI 1, oauth2-proxy 1 (singleton in every env) | hostnames, vault refs, proxy args |

## Updates

Policy `update-policies/flux-operator-ui.yaml` (UI marker `apps:flux-operator-ui:tag`, floor `>=0.60.0`; proxy markers shared with clickstack + hubble-ui, bump together). Changelog: [flux-operator](https://github.com/controlplaneio-fluxcd/flux-operator/releases).
