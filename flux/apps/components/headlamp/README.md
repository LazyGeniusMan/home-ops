# Headlamp (§13.3)

Kubernetes web UI, image `ghcr.io/headlamp-k8s/headlamp:v0.45.0`, with native
OIDC login via Zitadel (§11.1) plus the locked plugin set.

## Layout (environment-direct, apps area)

`base/` holds every manifest (`headlamp.yaml` OCIRepository + HelmRelease,
secrets, RBAC, wildcard certificate, HTTPRoute); env overlays
`{dev,prd}/` patch hostnames, vault refs, and OIDC issuer via
`resources: [../base]`. Tenant is `apps/headlamp` via
`flux/apps/update-policies/headlamp.yaml`.

## Chart source

OCI `oci://chartproxy.container-registry.com/kubernetes-sigs.github.io/headlamp/headlamp`,
tag **0.45.0** (classic `https://kubernetes-sigs.github.io/headlamp/`
upstream, chart `headlamp`, proxied to OCI via chartproxy).
No cosign `verify` block: proxied tarballs are live-translated by
chartproxy, so no upstream signature applies.

- Chart↔app lockstep (NOT the §11.1 Zitadel divergence): chart **0.45.0**
  carries app **0.45.0**. `image.tag` is pinned explicitly to `v0.45.0` with
  the `$imagepolicy` marker (`apps:headlamp:tag`); on automation PRs bump the
  OCIRepository `ref.tag` to match.

## OIDC (direct — NO oauth2-proxy)

Headlamp speaks OIDC natively (docs: `installation/in-cluster/oidc.md`).
The HelmRelease wires the chart's `externalSecret`
contract: the ESO-synced `headlamp-oidc` Secret provides `OIDC_CLIENT_ID`,
`OIDC_CLIENT_SECRET`, `OIDC_ISSUER_URL`, `OIDC_SCOPES` (with
`hasScopes: true` so `-oidc-scopes` is passed). No chart-managed Secret
(`config.oidc.secret.create: false`). Chart-native `httpRoute` stays off —
the route is owned explicitly (§8.1 pattern, same split as §11.1).

| Item | Value |
| --- | --- |
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| clientID | `headlamp` |
| Scopes | `openid profile email groups` |
| Callback | `https://headlamp.home-ops.yansyah.my.id/oidc-callback` (covered by the locked `https://headlamp.home-ops.yansyah.my.id/*` redirect) |

Client-secret choice: **confidential** — this app's `headlamp-sso` Terraform
module declares the `headlamp` client with
`auth_method_type = OIDC_AUTH_METHOD_TYPE_BASIC` (see `terraform/main.tf`),
so a secret is required; NOT public-client PKCE.

Secret handoff (stored outputs, end-to-end — NO pass:// seeding for OIDC
creds): the `headlamp-sso` module outputs the generated `client_id` +
`client_secret` into the CR output Secret `headlamp-sso-outputs` (CR
`writeOutputsToSecret`), and ESO `headlamp-oidc` consumes BOTH keys from that
Secret through the in-cluster `headlamp-k8s` SecretStore (ESO Kubernetes
provider: `eso-k8s-reader` SA + in-namespace Role/RoleBinding,
`remoteNamespace: headlamp`, same-cluster API via `kube-root-ca.crt`).
No `headlamp/oidc-client-*` vault entries exist or are needed; rotation is
automatic on the next `headlamp-sso` reconcile (refreshInterval 1h).

Prerequisite: the chart setup Job mints the IAM_OWNER machine key
and ESO mirrors both handoff Secrets (`zitadel-bootstrap-credentials` for
provider auth, `zitadel-bootstrap-outputs` for org_id + admin_user_id) into
`headlamp-terraform-vars` — no vault seeding, no console step, no pass://
dependency left for SSO. Without the mirrored Secret the CR retries on
interval.
Upstream identity (org_id + admin user ID) mirrors from the FirstInstance
handoff via the ESO-synced `headlamp-terraform-vars` Secret (same-namespace
`varsFrom` + cross-namespace `headlamp-zitadel` SecretStore) — no `org_id`
literal in git, no manual per-env fill, no email lookups, no remote-state
read. Normal users are
owned by this slice (`zitadel_human_user.users`, created from
`var.user_emails`; empty = admin-only).

## Plugins (pluginsManager sidecar, all pinned)

The plugin-manager sidecar resolves each `source` via ArtifactHub metadata
(`archive-url` + SHA-256 verify) and extracts into Headlamp's plugins dir;
`config.watchPlugins: true` hot-reloads updates without pod recreation.

| Plugin | Version | Source URL |
| --- | --- | --- |
| ai-assistant (`headlamp_ai_assistant`) | `0.4.1-alpha` | `https://artifacthub.io/packages/headlamp/headlamp-plugins/headlamp_ai_assistant` |
| flux (`headlamp_flux`) | `0.7.0` | `https://artifacthub.io/packages/headlamp/headlamp-plugins/headlamp_flux` |
| kubevirt (`headlamp_kubevirt`) | `0.3.1` | `https://artifacthub.io/packages/headlamp/headlamp-kubevirt/headlamp_kubevirt` |

Plugin sources: `headlamp-plugins` repo for ai-assistant/flux,
`headlamp-kubevirt` repo for kubevirt (per the plugin's README; contract:
`/tmp/home-ops-docs/headlamp-kubevirt-plugin-docs`). Bumps flow through
`update-policies/headlamp.yaml` + PR automation;
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

Group-subject form: subject names must match the API server's OIDC
userClaim/groupsClaim (same `groups` claim; see the RECONCILE note atop
`headlamp-rbac.yaml`).

## Routing / TLS (§8.1 pattern)

`base/headlamp-httproute.yaml`: `headlamp.home-ops.yansyah.my.id`,
catch-all `/` (covers app + `/oidc-callback`) → `headlamp:80` on the shared
`main` Gateway (cross-namespace parentRef). TLS terminates at the Gateway
via the in-namespace wildcard `Certificate` (`wildcard-certificate.yaml`,
same duplicate pattern as §§8.1/11.1 — cert-manager Secrets are
namespace-local).

## Credentials

`ExternalSecret/headlamp-oidc` syncs the client credentials from the
`headlamp-sso-outputs` Secret via the in-cluster `headlamp-k8s` SecretStore
(stored outputs, end-to-end — no Proton Pass seeding for OIDC creds; static
fields are templated, Git holds refs only). The Cloudflare token mirror in
`wildcard-certificate.yaml` uses the same vault path as §§9–10, copied so
the DNS-01 secret exists in this namespace too.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | `replicaCount` 1 (single-instance) | hostnames, vault refs, OIDC issuer + `replicaCount` → 1 |
| `prd` | `replicaCount` 2 (recommended production) | hostnames, vault refs, OIDC issuer + `replicaCount` → 2 |

Plugin set rides the same per-env patches.

Upstream reference (read-only): `/tmp/home-ops-docs/headlamp-docs/charts/headlamp`.

## Telemetry-off / monitoring / updates

- Telemetry evidence: the chart `values.yaml` contains no phone-home,
  analytics, or usage-reporting knobs; bundled static plugins are pure UI.
  Nothing to switch off.
- `ServiceMonitor: off` (same §9 deviation).
- App auto-tracks via `update-policies/headlamp.yaml`
  (`ghcr.io/headlamp-k8s/headlamp:v0.45.0` marker + chart version floor);
  plugin pins ride along in the same PR by hand.

## Upgrade runbook

- Version source: OCIRepository `ref.tag` + `image.tag` in
  `base/headlamp.yaml` (chart 0.45.0 == app 0.45.0 lockstep — NOT the
  §11.1 Zitadel divergence) plus the plugin pins in `configContent`
  (ai-assistant, flux, kubevirt).
- Changelog: https://github.com/headlamp-k8s/headlamp/releases
  (plugins via the ArtifactHub links in the Plugins table above).
- Bump: let the image ImagePolicy PR land (marker `apps:headlamp:tag`,
  `update-policies/headlamp.yaml`), then set the OCIRepository `ref.tag`
  to match AND hand-bump the plugin pins in the SAME PR (same file).
- Verify: OIDC login succeeds and the plugin list renders in the UI.
