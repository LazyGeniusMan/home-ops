# Coder (§13.6)

Self-hosted remote dev environments at
`https://coder.home-ops.yansyah.my.id`, app `v2.37.0` via chart
`coder-v2/coder` **2.37.0** (classic `https://helm.coder.com/v2` repo —
OCI pulls 403/404 on every upstream path, verified at authoring time, so
the chart pin is bumped manually while `ghcr.io/coder/coder` auto-tracks
through `update-policies/coder.yaml`; same split as the seaweedfs
component). Default upstream templates first — no custom workspace
template is authored here (`coder-templates`/`coder-modules` skills apply
only when a custom template is required; none is).

## Layout (environment-direct, apps area)

`base/` holds every manifest (`coder.yaml` HelmRepository + HelmRelease,
secrets, CNPG Cluster, wildcard certificates, HTTPRoutes); env overlays
`{dev,stg,prd}/` patch hostnames, vault refs, and chart values
via `resources: [../base]`. Tenant is `apps/coder` (wired by the fleet tenant
file, not here — no tenant/workflow edits in this change).

## OIDC (direct — NO oauth2-proxy)

Coder speaks OIDC natively against the §11.1 issuer. SSO is owned by THIS
app (per-app decoupling — the central zitadel module owns no clients):

- `terraform/` — owns the `coder` Zitadel project + project-scoped roles
  `coder-admin` / `coder-user` + user grants + the `coder` OIDC client.
  Upstream identity (org_id + admin/user IDs) flows from the zitadel
  bootstrap slice via `data.terraform_remote_state` (in-cluster Kubernetes
  backend, state Secret `tfstate-default-zitadel-bootstrap-identity` in the
  `zitadel` namespace) — no `org_id` var, no manual per-env fill, no email
  lookups. The read runs as the coder-namespace tofu runner SA, whose narrow
  cross-namespace grant (Role + RoleBinding in the zitadel namespace,
  get+list on the bootstrap state Secret only) ships in
  `base/terraform-remote-state-rbac.yaml` in this SAME base dir so it
  reconciles (and prunes) with the app. Machine-applied by the `coder-sso`
  Terraform CR in `base/terraform.yaml` (same shape as the zitadel bootstrap
  CR: OCI `apps` source, `./terraform` path, plain per-env `vars` + ESO
  `coder-terraform-vars` varsFrom, in-cluster state backend, `client_id` +
  `client_secret` in `coder-sso-outputs`). No `dependsOn` — fleet ordering
  (`apps` after `infra-configs`) is the mechanism.
- `coder-admin` is project-scoped: it NEVER implies org admin (ORG_OWNER
  stays with the bootstrap org membership only).

| Item | Value |
|---|---|
| Issuer | `https://zitadel.home-ops.yansyah.my.id` |
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

JWT prerequisite (one-time, manual): the `coder-terraform-vars`
ExternalSecret mirrors the shared instance key
`pass://<env-vault>/zitadel/terraform-jwt-profile-json` (same IAM_OWNER
service-user key the zitadel bootstrap uses — mirrored per namespace like
the cloudflare-api-token mirrors). That JWT key is the ONLY remaining
pass:// dependency for SSO. Without the key the CR retries on interval.

## SSO identity → operator mapping (LOCKED)

The §11.1 Zitadel users ARE the SSO identities; Coder derives its users
from their OIDC claims (email prefix → username when no
`preferred_username` claim):

| Zitadel SSO identity | Operator identity | Coder username | Groups |
|---|---|---|---|
| `admin@home-ops.yansyah.my.id` | `git@yansyah.my.id` | `admin` | `coder-admin` |
| `user@home-ops.yansyah.my.id` | `git@lazygeniusman.my.id` | `user` | `coder-user` |

First OIDC login claims instance ownership — perform it as
`admin@home-ops.yansyah.my.id` before inviting anyone else.

Follow-up (do NOT edit zitadel files here): if distinct `git@…` IdP
identities are later added in the zitadel terraform, extend this table and
re-check `CODER_OIDC_EMAIL_DOMAIN` / the group allowlist against the new
emails/groups. Related hardening after first login succeeds: set
`CODER_DISABLE_PASSWORD_AUTH=true` so OIDC is the only sign-in path
(left off in base so a misconfigured OIDC cannot lock out the instance).

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

Follow-up in the gateway-api component (out of scope here — this change
touches only coder paths): the shared Gateway's `https` listener
currently terminates with the top-level `*.home-ops.yansyah.my.id` cert,
which does not cover `*.coder.…`. Attach `coder-wildcard-tls` (e.g. an
additional `https` listener with that certificateRef, or a SNI-based
listener addition) so workspace hostnames terminate correctly.

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
`coder-terraform-vars` mirrors the shared instance JWT key — the only SSO
pass:// entry left), `coder-db-credentials` + `coder-db-app-secret`
(same-password pair, see `coder-secrets.yaml`), `cnpg-s3-credentials` +
`cloudflare-api-token`
(same vault paths as §§9–10, copied so Barman/DNS-01 secrets exist in
this namespace too). Seed each remaining vault entry with pass-cli.

## Environments

`prd` and `stg` inherit `../base` unchanged (same shape as
cert-manager before per-env divergence). Per-env tuning (replicas,
storage size, `CODER_DISABLE_PASSWORD_AUTH`) lands with the first real
divergence, not here.

## Telemetry-off / monitoring / updates

- Telemetry evidence: pulled chart `values.yaml` documents
  `telemetry.enable` defaulting `true`; this component overrides it with
  `CODER_TELEMETRY_ENABLE=false` in the HelmRelease env.
- Unguarded monitors OFF: the chart ships no ServiceMonitor objects,
  none are added here, and `CODER_PROMETHEUS_ENABLE` stays unset
  (default off) until `monitoring.coreos.com` CRDs land (same §9
  deviation). Coderd health via `kube-state-metrics` meanwhile.
- App image auto-tracks via `update-policies/coder.yaml`
  (`ghcr.io/coder/coder:v2.37.0` marker); chart bumps are manual.
