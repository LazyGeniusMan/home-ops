# Headlamp

Kubernetes web UI, image `ghcr.io/headlamp-k8s/headlamp:v0.45.0`, with native
OIDC login via Zitadel plus the locked plugin set.

## Layout

`base/` holds every manifest (`headlamp.yaml` OCIRepository + HelmRelease,
secrets, RBAC, wildcard certificate, HTTPRoute); env overlays `{dev,prd}/`
patch hostnames, vault refs, and OIDC issuer via `resources: [../base]`.
Tenant is `apps/headlamp` via `flux/apps/update-policies/headlamp.yaml`.

## Chart source

OCI `oci://chartproxy.container-registry.com/kubernetes-sigs.github.io/headlamp/headlamp`,
tag **0.45.0** (proxy of classic `https://kubernetes-sigs.github.io/headlamp/`).
No cosign `verify` block (chartproxy live-translates tarballs, so no upstream
signature applies). Chart **0.45.0** carries app **0.45.0**; `image.tag` is
pinned to `v0.45.0` with the `$imagepolicy` marker (`apps:headlamp:tag`) —
bump the OCIRepository `ref.tag` to match.

## OIDC (direct — NO oauth2-proxy)

Headlamp speaks OIDC natively. The ESO-synced `headlamp-oidc` Secret provides
`OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_ISSUER_URL`, `OIDC_SCOPES`
(`hasScopes: true`). No chart-managed Secret
(`config.oidc.secret.create: false`); chart-native `httpRoute` stays off.

| Item | Value |
| --- | --- |
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| clientID | `headlamp` (confidential — secret required) |
| Scopes | `openid profile email groups` |
| Callback | `https://headlamp.home-ops.yansyah.my.id/oidc-callback` (covered by `https://headlamp.home-ops.yansyah.my.id/*`) |

Secret handoff (no pass:// seeding): `headlamp-sso` outputs `client_id` +
`client_secret` into `headlamp-sso-outputs`, consumed by ESO `headlamp-oidc`
via `headlamp-k8s`. Upstream identity mirrors from the FirstInstance handoff
via `headlamp-terraform-vars` — no `org_id` literal in git. Normal users via
`var.user_emails` (empty = admin-only).

## Plugins (pluginsManager sidecar, all pinned)

`config.watchPlugins: true` hot-reloads updates without pod recreation.
Pinned set (see `base/headlamp.yaml` `configContent`): ai-assistant
(`headlamp_ai_assistant` 0.4.1-alpha), flux (`headlamp_flux` 0.7.0), kubevirt
(`headlamp_kubevirt` 0.3.1). Plugin pins hand-bump in the same PR as the
image bump.

## RBAC mapping

Headlamp forwards the user's OIDC token (no claims-mapping knob), so the
mapping is explicit bindings in `base/headlamp-rbac.yaml` against the
`groups` claim: `admin` group → `cluster-admin` (`headlamp-admins`);
`users` group → `view` + `headlamp-basic` (self-review + namespace list so the
UI enumerates contexts). Subject names must match the API server's OIDC
userClaim/groupsClaim.

## Routing / TLS

`base/headlamp-httproute.yaml`: `headlamp.home-ops.yansyah.my.id`,
catch-all `/` (covers app + `/oidc-callback`) → `headlamp:80` on the shared
`main` Gateway (cross-namespace parentRef). TLS via the in-namespace wildcard
`Certificate` (cert-manager Secrets are namespace-local); the Cloudflare token
mirror copies the DNS-01 secret into this namespace.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | `replicaCount` 1 | hostnames, vault refs, OIDC issuer + `replicaCount` → 1 |
| `prd` | `replicaCount` 2 | hostnames, vault refs, OIDC issuer + `replicaCount` → 2 |

Upstream reference (read-only): `/tmp/home-ops-docs/headlamp-docs/charts/headlamp`.

## Telemetry / monitoring / updates

No telemetry knobs; `ServiceMonitor: off`. Auto-tracks via
`update-policies/headlamp.yaml` (marker + chart floor); plugin pins ride
along in the same PR by hand. Version source: OCIRepository `ref.tag` +
`image.tag` in `base/headlamp.yaml` (chart 0.45.0 == app 0.45.0) plus plugin
pins in `configContent`.
Changelog: https://github.com/headlamp-k8s/headlamp/releases (plugins via the
ArtifactHub links in `base/headlamp.yaml`).
