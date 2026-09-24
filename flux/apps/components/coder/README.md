# Coder (§13.6)

Self-hosted remote dev environments at
`https://coder.home-ops.yansyah.my.id`, app `v2.37.3` via chart
`oci://ghcr.io/coder/chart/coder` **2.37.3** (OCI on GHCR is the upstream
source of truth — digest
`sha256:922fa45fae4cb2e2cb92d73fb0327878cc84177c3c701affa5cfb866706b4d50`,
appVersion 2.37.3, verified by pull; chart<->app lockstep, so the chart
tag and `ghcr.io/coder/coder` image tag auto-track together through
`update-policies/coder.yaml`). Default upstream templates first — no
custom workspace template is authored here
(`coder-templates`/`coder-modules` skills apply only when a custom
template is required; none is).

## Layout (environment-direct, apps area)

`base/` holds every manifest (`coder.yaml` OCIRepository + HelmRelease,
secrets, CNPG Cluster, wildcard certificates, HTTPRoutes); env overlays
`{dev,prd}/` patch hostnames, vault refs, and chart values
via `resources: [../base]`. Tenant is `apps/coder` (wired in
`flux/fleet/tenants/apps.yaml`).

## OIDC (direct — NO oauth2-proxy)

Coder speaks OIDC natively against the §11.1 issuer. SSO is owned by THIS
app (per-app decoupling — the central zitadel module owns no clients):

- `terraform/` — owns the `coder` Zitadel project + project-scoped roles
  `coder-admin` / `coder-user` + user grants + the `coder` OIDC client +
  the normal users themselves (`zitadel_human_user.users`, created from
  `var.user_emails`; empty = admin-only).
  Upstream identity (org_id + admin user ID) flows from the FirstInstance
  handoff (`zitadel-bootstrap-outputs` Secret in the `zitadel` namespace,
  operator-created once per the zitadel README runbook) via the ESO-synced
  `coder-terraform-vars` Secret (same-namespace `varsFrom` — a cross-namespace
  `coder-zitadel` SecretStore + the narrow `coder-zitadel-handoff-reader` Role
  in `base/zitadel-handoff-rbac.yaml` do the mirroring) — no `org_id` literal
  in git, no manual per-env fill, no email lookups, no remote-state read.
  Provider auth (`jwt_profile_json`) mirrors from the chart-kept
  `zitadel-bootstrap-credentials` through the same Secret. Machine-applied by
  the `coder-sso` Terraform CR in `base/terraform.yaml` (OCI `apps` source,
  `./terraform` path, plain per-env `vars` + ESO `coder-terraform-vars`
  varsFrom, in-cluster state backend, `client_id` + `client_secret` in
  `coder-sso-outputs`). No `dependsOn` — fleet ordering (`apps` after
  `infra-configs`) is the mechanism.
- `coder-admin` is project-scoped: it NEVER implies org admin (ORG_OWNER
  stays with the bootstrap org membership only).

| Item | Value |
|---|---|
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Client | `coder` (owned here; `client_id` is generated server-side — synced via ESO, never the literal name) |
| Redirect | `https://coder.home-ops.yansyah.my.id/*` (covers the callback `/api/v2/users/oidc/callback`) |
| Scopes | `openid,profile,email,groups` |
| Email domain | `home-ops.yansyah.my.id` |
| Group allowlist | `coder-admin,coder-user` (`CODER_OIDC_ALLOWED_GROUPS`; the `groups` claim needs no `groupField` override) |

Role matrix:

| Zitadel SSO identity | Coder role mapping | Groups claim |
|---|---|---|
| `admin@home-ops.yansyah.my.id` (super-admin) | instance owner (first login claims ownership) | `coder-admin` |
| `user@home-ops.yansyah.my.id` (normal) | regular user | `coder-user` |

Both groups sign in; Coder-side ownership/RBAC distinguishes them (first
OIDC login claims instance ownership — perform it as admin@ before
inviting anyone else).

Secret handoff (stored outputs, end-to-end — NO pass:// seeding for OIDC
creds): the `coder-sso` module outputs the generated `client_id` +
`client_secret` into the CR output Secret `coder-sso-outputs` (CR
`writeOutputsToSecret`), and ESO `coder-oidc` consumes BOTH keys from that
Secret through the in-cluster `coder-k8s` SecretStore (ESO Kubernetes
provider: `eso-k8s-reader` SA + in-namespace Role/RoleBinding,
`remoteNamespace: coder`, same-cluster API via `kube-root-ca.crt` —
first in-repo usage of the Kubernetes provider; the repo otherwise only has
the `proton-pass` ClusterSecretStore). The HelmRelease consumes them via
`secretKeyRef` (`CODER_OIDC_CLIENT_ID` ← `client-id`,
`CODER_OIDC_CLIENT_SECRET` ← `client-secret`). No `coder/oidc-client-*`
vault entries exist or are needed; rotation is automatic on the next
`coder-sso` reconcile (refreshInterval 1h).

Zero-UI prerequisite: the chart setup Job mints the IAM_OWNER machine key
and ESO mirrors both handoff Secrets (`zitadel-bootstrap-credentials` for
provider auth, `zitadel-bootstrap-outputs` for org_id + admin_user_id) into
`coder-terraform-vars` — no vault seeding, no console step, no pass://
dependency left for SSO. Without the mirrored Secret the CR retries on
interval.

## SSO identity → operator mapping (LOCKED)

The §11.1 Zitadel users ARE the SSO identities; Coder derives its users
from their OIDC claims (email prefix → username when no
`preferred_username` claim):

| Zitadel SSO identity | Operator identity | Coder username | Groups |
|---|---|---|---|
| `admin@home-ops.yansyah.my.id` | `git@yansyah.my.id` | `admin` | `coder-admin` |
| `git@yansyah.my.id` | `git@yansyah.my.id` | `git` | `coder-user` |
| `git@lazygeniusman.my.id` | `git@lazygeniusman.my.id` | `git` | `coder-user` |

First OIDC login claims instance ownership — perform it as
`admin@home-ops.yansyah.my.id` before inviting anyone else. Note: both
`git@…` addresses share the email prefix `git`, so the default
email-prefix username derivation collides — verify on the second user's
first login and, if Coder rejects the duplicate username, set
`CODER_OIDC_USERNAME_FIELD=email` in `base/coder.yaml`.

Membership is deliberately env-invariant: `base/terraform.yaml`
`user_emails` grants the same humans in dev+prd (no overlay patches) —
this table is the source of truth for who those humans are.

If distinct `git@…` IdP identities are added in the zitadel terraform,
extend this table and re-check `CODER_OIDC_EMAIL_DOMAIN` / the group
allowlist against the new emails/groups. Related hardening after first login
succeeds: set `CODER_DISABLE_PASSWORD_AUTH=true` so OIDC is the only sign-in
path (left off in base so a misconfigured OIDC cannot lock out the instance).

## Routing (dashboard + workspace wildcard)

Hand-written HTTPRoutes on the shared §8.1 Gateway (`main`,
cross-namespace parentRef); chart-native routing stays off:

- `coder` (`coder.home-ops.yansyah.my.id`, `/`) → `coder:80` (UI, API,
  OIDC callback).
- `coder-workspaces` (`*.coder.home-ops.yansyah.my.id`, `/`) → same
  Service — coderd multiplexes `<workspace>.coder.…` by subdomain
  (`CODER_WILDCARD_ACCESS_URL`), so one route covers every workspace and
  nested app subdomains; no per-workspace objects.
- `coder-redirect` / `coder-workspaces-redirect`: HTTP→HTTPS 301s on the
  `http` listener (same shape as §8.1 `redirect-services`).

Workspace-agent connectivity: agents dial the wildcard hostname through
the Gateway, which terminates TLS (see below) and forwards to coderd —
no agent-to-pod networking outside the Gateway path is required.

## TLS + DNS

Two in-namespace Certificates (namespace-local pattern, same as §8.1):
`coder-root` (`coder.home-ops.yansyah.my.id` → `coder-tls`) and
`coder-wildcard` (`*.coder.home-ops.yansyah.my.id` →
`coder-wildcard-tls`), both via `ClusterIssuer/letsencrypt` DNS-01 (the
`cloudflare-api-token` ExternalSecret mirrors the §9 remoteRef). A
single-label wildcard cannot cover the nested workspace shape, hence two
certs. No manual DNS: issuance uses DNS-01 TXT, and A records ride on the
§9.4 external-dns automation.

Gateway note: the shared Gateway's `https` listener terminates with the
top-level `*.home-ops.yansyah.my.id` cert, which does not cover
`*.coder.…`. Attach `coder-wildcard-tls` (e.g. an additional `https`
listener with that certificateRef, or an SNI-based listener addition) so
workspace hostnames terminate correctly.

## Database

`base/coder-db.yaml` — namespace-local instantiation of the
§10.1 `cluster-base` template (3 instances, sync quorum 1,
`local-ssd-nvme`, continuous WAL + daily base backup to SeaweedFS S3 under
`s3://cnpg-backups/coder/`). Adjusted: dbname/owner `coder`.
`CODER_PG_CONNECTION_URL` reads the `coder-db-credentials` ExternalSecret
(`sslmode=require`, same self-signed trade-off as zitadel).

## Credentials

`ExternalSecret/coder-oidc` (Zitadel client id + secret from stored outputs
via the `coder-k8s` SecretStore — see the OIDC secret-handoff runbook above;
`coder-terraform-vars` mirrors the FirstInstance handoff — no SSO
pass:// entry left), `coder-db-credentials` + `coder-db-app-secret`
(same-password pair, see `coder-secrets.yaml`), `cnpg-s3-credentials` +
`cloudflare-api-token`. `cnpg-s3-credentials` (coder-db) is COSI-minted:
it syncs from the `coder-db-cosi-creds` BucketInfo JSON through the
in-namespace `coder-cosi` SecretStore (dedicated claim `coder-db` — see
`base/bucketclaims.yaml` and the cosi README).
`cloudflare-api-token` follows the §9 vault path so the DNS-01 secret
exists in this namespace too. Seed each vault entry with
pass-cli.

Coder-owned Matrix notifier (BOOTSTRAPPED — no matrix-tenant fallback leg,
no `apprise-coder` Provider; see matrix `base/NOTIFICATIONS.md` ownership
table): `ExternalSecret/matrix-notify` composes TWO keys from the matrix
bot bootstrap kept Secret in ESO `target.template` (cross-namespace
`coder-matrix` SecretStore — zitadel-consumer pattern; zero vault seeding):

| Kept-Secret key (`matrix-bot-bootstrap-outputs`, ns `matrix`) | Secret key | Consumed by | Value notes |
|---|---|---|---|
| `notifier-token` | `matrix-notify` → `matrix-notify` (`apprise-urls` bare + `webhook-endpoint` full) | `CODER_MATRIX_APPRISE_URLS` + `CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT` (valueFrom.secretKeyRef in `base/coder.yaml`) | The SAME per-env bot token that delivers to every room (`@apprise-dev` dev / `@apprise` prd); only rooms differ |
| `homeserver-host` | (same ES/Secret) | (same — the `<host>` half of the composed `matrixs://` URL) | Bare host, NO scheme (dev `tuwunel.matrix.home-ops-dev.yansyah.my.id`, prd `tuwunel.matrix.home-ops.yansyah.my.id` — minted by the Job, never Git) |

The notifier credential flows Job -> kept Secret -> this ES cross-namespace
(narrow `coder-matrix-handoff-reader` Role/Binding in ns `matrix` +
`coder-matrix` SecretStore in `base/coder-secrets.yaml`).

Data-flow: Job -> kept Secret `matrix-bot-bootstrap-outputs` (ns
`matrix`) -> ES `matrix-notify` (coder-matrix store) -> Secret
`matrix-notify` -> HelmRelease env -> sink per-request `urls` (body).
Current limitation (see matrix NOTIFICATIONS.md): the sink reads `urls`
from the POST body only and coderd's payload is fixed, so Coder posts
return 204 without a visible message.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | coderd `replicaCount` 1, `coder-db` Cluster 1 (single-instance) | vault refs, hostnames, chart values + `replicaCount` → 1, `instances` → 1 |
| `prd` | coderd `replicaCount` 2, `coder-db` Cluster 3 (recommended production) | vault refs, hostnames, chart values + `replicaCount` → 2, `instances` → 3 |

Storage size and `CODER_DISABLE_PASSWORD_AUTH` ride the same per-env
patches once they diverge. Rclone sync (`rclone-sync-coder-db`): 1 per
instance/schedule, `concurrencyPolicy: Forbid` — no scaling.

Upstream reference (read-only): `/tmp/home-ops-docs/coder-docs`.

## Telemetry-off / monitoring / updates

- Telemetry evidence: pulled chart `values.yaml` documents
  `telemetry.enable` defaulting `true`; this component overrides it with
  `CODER_TELEMETRY_ENABLE=false` in the HelmRelease env.
- Unguarded monitors OFF: the chart ships no ServiceMonitor objects,
  none are added here, and `CODER_PROMETHEUS_ENABLE` stays unset
  (default off) until `monitoring.coreos.com` CRDs land (same §9
  deviation). Coderd health via `kube-state-metrics` meanwhile.
- Chart tag + app image auto-track together via `update-policies/coder.yaml`
  (chart marker `apps:coder-chart:tag` on the OCIRepository,
  `ghcr.io/coder/coder:v2.37.3` app marker `apps:coder:tag` — chart<->app
  lockstep, bump both together).

## Upgrade runbook

- Version source: OCI chart tag in `base/coder.yaml`
  (`oci://ghcr.io/coder/chart/coder:2.37.3`, marker `apps:coder-chart:tag`,
  auto) + app image tag in the same file
  (`ghcr.io/coder/coder:v2.37.3`, marker `apps:coder:tag`, auto) —
  chart<->app lockstep (chart 2.37.3 embeds app v2.37.3).
- Changelog (app+chart): https://github.com/coder/coder/releases.
- Bump: let the two ImagePolicy PRs land (markers `apps:coder-chart:tag` +
  `apps:coder:tag`, `update-policies/coder.yaml`); take both together in
  the same PR when the release notes call for it — never one side alone.
- Migrate: snapshot `coder-db` BEFORE major bumps (fresh CNPG base
  backup — see the infra cnpg README restore runbook). Verify:
  dashboard OIDC login succeeds and a workspace agent connects via the
  `*.coder` wildcard route.
