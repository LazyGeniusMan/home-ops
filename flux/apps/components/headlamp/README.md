# Headlamp

Kubernetes web UI, image `ghcr.io/headlamp-k8s/headlamp:v0.45.0`, with native OIDC login via Zitadel plus the locked plugin set.

## Layout

`base/` holds every manifest (`headlamp.yaml`, secrets, RBAC, wildcard certificate, HTTPRoute); `{dev,prd}/` patch hostnames, vault refs, and OIDC issuer.

## Chart source

OCI `oci://chartproxy.container-registry.com/kubernetes-sigs.github.io/headlamp/headlamp`, tag **0.45.0** (proxy of classic `https://kubernetes-sigs.github.io/headlamp/`; no cosign `verify` — chartproxy live-translates tarballs). Chart 0.45.0 carries app 0.45.0; `image.tag` `v0.45.0` carries the `$imagepolicy` marker (`apps:headlamp:tag`).

## OIDC (direct — no oauth2-proxy)

ESO-synced `headlamp-oidc` Secret provides `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_ISSUER_URL`, `OIDC_SCOPES`. Chart-native `httpRoute` stays off.

| Item | Value |
| --- | --- |
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| clientID | `headlamp` (confidential) |
| Scopes | `openid profile email groups` |
| Callback | `https://headlamp.home-ops.yansyah.my.id/oidc-callback` |

## Credentials

`headlamp-oidc` from `headlamp-sso-outputs` (Terraform-owned via `headlamp-k8s` store, no pass:// seeding, no `org_id` literal in git).

## Plugins

Pinned set in `base/headlamp.yaml` `configContent`: ai-assistant (`headlamp_ai_assistant` 0.4.1-alpha), flux (`headlamp_flux` 0.7.0), kubevirt (`headlamp_kubevirt` 0.3.1). `config.watchPlugins: true` hot-reloads updates. Plugin pins hand-bump in the same PR as the image bump.

## RBAC mapping

Headlamp forwards the user's OIDC token, so mapping is explicit bindings in `base/headlamp-rbac.yaml`: `admin` group → `cluster-admin` (ACCEPTED RISK — UI gated by native OIDC, no anonymous path); `users` group → `view` + `headlamp-basic`. `headlamp-ops` + `headlamp-ops-global-read` ClusterRoles ship UNBOUND as the least-privilege path.

## Routing

`headlamp.home-ops.yansyah.my.id`, `/` → `headlamp:80` on shared `main` Gateway. TLS via the in-namespace wildcard Certificate.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | `replicaCount` 1 | hostnames, vault refs, OIDC issuer |
| `prd` | `replicaCount` 2 | hostnames, vault refs, OIDC issuer |

## Updates

Policy `update-policies/headlamp.yaml` (marker + chart floor; plugin pins ride along by hand). Changelog: [headlamp](https://github.com/headlamp-k8s/headlamp/releases) (plugins via ArtifactHub links in `base/headlamp.yaml`).
