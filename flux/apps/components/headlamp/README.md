# Headlamp (§13.3)

Kubernetes web UI, image `ghcr.io/headlamp-k8s/headlamp:v0.45.0`, with native
OIDC login via Zitadel (§11.1) plus the locked plugin set.

## Layout (environment-direct, apps area)

`base/` holds every manifest (`headlamp.yaml` HelmRepository + HelmRelease,
secrets, RBAC, wildcard certificate, HTTPRoute); env overlays
`{dev,stg,prd}/` patch hostnames, vault refs, and OIDC issuer via
`resources: [../base]`. Tenant is `apps/headlamp` via
`flux/apps/update-policies/headlamp.yaml`.

## Chart source

Classic HelmRepository `https://kubernetes-sigs.github.io/headlamp/`, chart
**0.45.0** (verified in the repo index at authoring time; `helm template` +
`helm lint` pass locally against the chart copy in
`/tmp/home-ops-docs/headlamp-docs/charts/headlamp`).

- Chart↔app lockstep (NOT the §11.1 Zitadel divergence): chart **0.45.0**
  carries app **0.45.0**. `image.tag` is pinned explicitly to `v0.45.0` with
  the `$imagepolicy` marker (`apps:headlamp:tag`); on automation PRs bump the
  chart `version:` to match.
- No OCI fallback: no OCI artifact is published for this chart (`helm pull
  oci://ghcr.io/headlamp-k8s/charts/headlamp --version 0.45.0` → 403 denied
  at authoring time). Same OCI-first deviation as §8.3 CoreDNS.
- App image verified live on GHCR (`ghcr.io/headlamp-k8s/headlamp:v0.45.0`
  tag listed at authoring time).

## OIDC (direct — NO oauth2-proxy)

Headlamp speaks OIDC natively (docs: `installation/in-cluster/oidc.md` in
the bounded sources). The HelmRelease wires the chart's `externalSecret`
contract: the ESO-synced `headlamp-oidc` Secret provides `OIDC_CLIENT_ID`,
`OIDC_CLIENT_SECRET`, `OIDC_ISSUER_URL`, `OIDC_SCOPES` (with
`hasScopes: true` so `-oidc-scopes` is passed). No chart-managed Secret
(`config.oidc.secret.create: false`). Chart-native `httpRoute` stays off —
the route is owned explicitly (§8.1 pattern, same split as §11.1).

| Item | Value |
| --- | --- |
| Issuer | `https://zitadel.home-ops.yansyah.my.id` |
| clientID | `headlamp` |
| Scopes | `openid profile email groups` |
| Callback | `https://headlamp.home-ops.yansyah.my.id/oidc-callback` (covered by the locked `https://headlamp.home-ops.yansyah.my.id/*` redirect) |

Client-secret choice: **confidential** — the §11.1 Tofu module declares the
`headlamp` client with `auth_method_type = OIDC_AUTH_METHOD_TYPE_BASIC`
(see `flux/infra/components/zitadel/terraform/main.tf`), so a secret is
required; NOT public-client PKCE. The secret is read out of the Tofu state
after `terraform apply` into
`pass://acme-prd-bdo1-talos-apps-01/headlamp/oidc-client-secret` (never Git).

## Plugins (pluginsManager sidecar, all pinned)

The plugin-manager sidecar resolves each `source` via ArtifactHub metadata
(`archive-url` + SHA-256 verify) and extracts into Headlamp's plugins dir;
`config.watchPlugins: true` hot-reloads updates without pod recreation.

| Plugin | Version | Source URL |
| --- | --- | --- |
| ai-assistant (`headlamp_ai_assistant`) | `0.4.0-alpha` | `https://artifacthub.io/packages/headlamp/headlamp-plugins/headlamp_ai_assistant` |
| flux (`headlamp_flux`) | `0.7.0` | `https://artifacthub.io/packages/headlamp/headlamp-plugins/headlamp_flux` |
| kubevirt (`headlamp_kubevirt`) | `0.3.1` | `https://artifacthub.io/packages/headlamp/headlamp-kubevirt/headlamp_kubevirt` |

Versions verified against the ArtifactHub API at authoring time
(`headlamp-plugins` repo for ai-assistant/flux; `headlamp-kubevirt` repo for
kubevirt — the plugin's own README prescribes exactly this source + version
pin). Bumps flow through `update-policies/headlamp.yaml` + PR automation;
the automation tracks the app image — plugin pins are bumped by hand in the
`configContent` block alongside (same file, same PR).

## RBAC mapping (LOCKED)

Headlamp forwards the user's OIDC token to the API server — it has NO
claims-mapping knob (the chart exposes only
`clusterRoleBinding.clusterRoleName` for the pod's own in-cluster `main`
context, left at `cluster-admin`). So the mapping is explicit bindings in
`base/headlamp-rbac.yaml` against the `groups` claim
(`OIDC_SCOPES` includes `groups`):

- `admin` group → `cluster-admin` (`headlamp-admins` ClusterRoleBinding).
- `users` group → `view` (`headlamp-users-view`) + `headlamp-basic`
  (self-review + namespace list so the UI enumerates contexts).

Group-subject form: future Zitadel group members inherit without manifest
edits. PRE-GO-LIVE: reconcile the subject names with the API server's OIDC
flags (structured auth userClaim/groupsClaim + prefixes — e.g. a `zitadel:`
prefix); the claim the API server must assert is the same `groups` claim
(see the RECONCILE note atop `headlamp-rbac.yaml`).

## Routing / TLS (§8.1 pattern)

`base/headlamp-httproute.yaml`: `headlamp.home-ops.yansyah.my.id`,
catch-all `/` (covers app + `/oidc-callback`) → `headlamp:80` on the shared
`main` Gateway (cross-namespace parentRef). TLS terminates at the Gateway
via the in-namespace wildcard `Certificate` (`wildcard-certificate.yaml`,
same duplicate pattern as §§8.1/11.1 — cert-manager Secrets are
namespace-local).

## Credentials

`ExternalSecret/headlamp-oidc` syncs the client secret from Proton Pass
(`pass://acme-prd-bdo1-talos-apps-01/headlamp/oidc-client-secret`; static
fields are templated, Git holds `remoteRef`s only). Seed the vault entry
with pass-cli. The Cloudflare token mirror in `wildcard-certificate.yaml`
uses the same vault path as §§9–10, copied so the DNS-01 secret exists in
this namespace too.

## Environments

`prd` and `stg` currently inherit `../base` unchanged (same shape
as cert-manager before per-env divergence). Per-env tuning (replicas,
plugin set, ExternalDomain hostnames) lands with the first real divergence,
not here.

## Telemetry-off / monitoring / updates

- Telemetry evidence: the chart `values.yaml` contains no phone-home,
  analytics, or usage-reporting knobs (checked at authoring time); bundled
  static plugins are pure UI. Nothing to switch off.
- Unguarded monitors OFF: no `ServiceMonitor` objects are shipped until
  `monitoring.coreos.com` CRDs land (same §9 deviation). Pod health via
  kube-state-metrics once the monitoring stack lands.
- App auto-tracks via `update-policies/headlamp.yaml`
  (`ghcr.io/headlamp-k8s/headlamp:v0.45.0` marker + chart version floor);
  plugin pins ride along in the same PR by hand.
