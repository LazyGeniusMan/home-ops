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
`{dev,staging,production}/` patch hostnames, vault refs, and chart values
via `resources: [../base]`. Tenant is `apps/coder` (wired by the fleet tenant
file, not here — no tenant/workflow edits in this change).

## OIDC (direct — NO oauth2-proxy)

Coder speaks OIDC natively against the §11.1 issuer:

| Item | Value |
|---|---|
| Issuer | `https://zitadel.home-ops.yansyah.my.id` |
| Client ID | `coder` (locked redirect `https://coder.home-ops.yansyah.my.id/*` covers the callback `/api/v2/users/oidc/callback`) |
| Scopes | `openid,profile,email,groups` |
| Email domain | `home-ops.yansyah.my.id` |
| Group allowlist | `admin,users` (`CODER_OIDC_ALLOWED_GROUPS`; the `groups` claim needs no `groupField` override) |

## SSO identity → operator mapping (LOCKED)

The §11.1 Zitadel users ARE the SSO identities; Coder derives its users
from their OIDC claims (email prefix → username when no
`preferred_username` claim):

| Zitadel SSO identity | Operator identity | Coder username | Groups |
|---|---|---|---|
| `admin@home-ops.yansyah.my.id` | `git@yansyah.my.id` | `admin` | `admin` (+users) |
| `user@home-ops.yansyah.my.id` | `git@lazygeniusman.my.id` | `user` | `users` |

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

`ExternalSecret/coder-oidc` (Zitadel client secret, read from terraform
state into `pass://acme-prd-bdo1-talos-apps-01/coder/oidc-client-secret`),
`coder-db-credentials` + `coder-db-app-secret` (same-password pair, see
`coder-secrets.yaml`), `cnpg-s3-credentials` + `cloudflare-api-token`
(same vault paths as §§9–10, copied so Barman/DNS-01 secrets exist in
this namespace too). Seed each vault entry with pass-cli.

## Environments

`production` and `staging` inherit `../base` unchanged (same shape as
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
