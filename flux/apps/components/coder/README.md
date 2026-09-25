# Coder

Self-hosted remote dev environments at `https://coder.home-ops.yansyah.my.id`, app `v2.37.3` via chart `oci://ghcr.io/coder/chart/coder` **2.37.3** (digest `sha256:922fa45fae4cb2e2cb92d73fb0327878cc84177c3c701affa5cfb866706b4d50`; chart<->app lockstep — chart tag and `ghcr.io/coder/coder` image tag track together via `update-policies/coder.yaml`). No custom workspace template.

## Layout

`base/` holds every manifest (`coder.yaml` OCIRepository + HelmRelease, secrets, CNPG Cluster, wildcard certificates, HTTPRoutes); env overlays `{dev,prd}/` patch hostnames, vault refs, and chart values via `resources: [../base]`.

## OIDC (direct — no oauth2-proxy)

Coder speaks OIDC natively. SSO is owned by this app: the `coder-sso` Terraform CR (`base/terraform.yaml`) owns the `coder` Zitadel project + `coder-admin` / `coder-user` roles + grants + OIDC client + users from `var.user_emails` (empty = admin-only; env-invariant — same humans in dev+prd).

| Item | Value |
|---|---|
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Client | `coder` (server-generated — synced via ESO, never a literal) |
| Redirect | `https://coder.home-ops.yansyah.my.id/*` (covers `/api/v2/users/oidc/callback`) |
| Scopes / domain / groups | `openid,profile,email,groups` / `home-ops.yansyah.my.id` / `coder-admin,coder-user` (`CODER_OIDC_ALLOWED_GROUPS`) |
| Identity map | `admin@…` → `admin`/`coder-admin`; `git@yansyah.my.id`, `git@lazygeniusman.my.id` → `git`/`coder-user` |

`coder-admin` is project-scoped — never implies org admin. First OIDC login claims instance ownership — perform it as `admin@home-ops.yansyah.my.id` first. Both `git@…` addresses share the email prefix `git`, so username derivation may collide — if Coder rejects the second login, set `CODER_OIDC_USERNAME_FIELD=email`. After first login succeeds, set `CODER_DISABLE_PASSWORD_AUTH=true` so OIDC is the only sign-in path (left off in base to avoid lockout). Secret handoff (stored outputs, no vault seeding): `coder-sso` outputs `client_id` + `client_secret` into `coder-sso-outputs`; ESO `coder-oidc` consumes both via the in-cluster `coder-k8s` SecretStore. `org_id` + admin ID + provider auth mirror from the FirstInstance handoff via `coder-terraform-vars` (RBAC in `zitadel-handoff-rbac.yaml`) — no `org_id` literal in git.

## Routing

Hand-written HTTPRoutes on the shared `main` Gateway (cross-namespace parentRef; chart-native routing stays off): `coder` (`coder.home-ops.yansyah.my.id`, `/`) → `coder:80` (UI, API, OIDC callback); `coder-workspaces` (`*.coder.home-ops.yansyah.my.id`, `/`) → same Service (coderd multiplexes by subdomain — one route covers all workspaces incl. nested subdomains); plus HTTP→HTTPS 301s for both. Agents dial the wildcard hostname through the Gateway; no agent-to-pod path outside it is required.

## TLS + DNS

Two in-namespace Certificates (cert-manager Secrets are namespace-local): `coder-root` (`coder.home-ops.yansyah.my.id` → `coder-tls`) and `coder-wildcard` (`*.coder.home-ops.yansyah.my.id` → `coder-wildcard-tls`), both via `ClusterIssuer/letsencrypt` DNS-01 (two certs: one wildcard covers a single label only). Issuance uses DNS-01 TXT; A records ride external-dns. Workspace hostnames need an additional `https` Gateway listener with the `coder-wildcard-tls` certificateRef before they terminate correctly.

## Database

`base/coder-db.yaml` — `coder` CNPG Cluster (3 instances, sync quorum 1, `local-ssd-nvme`, WAL + daily base backup to `s3://cnpg-backups/coder/`, dbname/owner `coder`). `CODER_PG_CONNECTION_URL` reads `coder-db-credentials` (`sslmode=require` — self-signed cert, still encrypted in transit).

## Credentials

`coder-oidc` (stored outputs via `coder-k8s`), `coder-db-credentials` + `coder-db-app-secret` (single `.../coder/db-password` vault source), `cnpg-s3-credentials` (COSI-minted via `coder-cosi`, claim `coder-db`) + `cloudflare-api-token` (DNS-01 secret in this namespace). Seed vault entries with pass-cli. Matrix notifier (coder-owned, no matrix-tenant leg): `matrix-notify` composes `webhook-endpoint` + `apprise-urls` from the matrix kept Secret via `coder-matrix` (zero vault seeding). Known gap: the apprise sink reads `urls` from the POST body only and coderd's payload is fixed, so posts return 204 with no message.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | coderd `replicaCount` 1, `coder-db` Cluster 1 | vault refs, hostnames, chart values + `replicaCount` → 1, `instances` → 1 |
| `prd` | coderd `replicaCount` 2, `coder-db` Cluster 3 | vault refs, hostnames, chart values + `replicaCount` → 2, `instances` → 3 |

Rclone sync (`rclone-sync-coder-db`): 1 per instance/schedule,
`concurrencyPolicy: Forbid` — no scaling.

Upstream reference (read-only): `/tmp/home-ops-docs/coder-docs`.

## Telemetry / monitoring / updates

- Telemetry off: `CODER_TELEMETRY_ENABLE=false`. No ServiceMonitor until monitoring CRDs land (`CODER_PROMETHEUS_ENABLE` unset; health via `kube-state-metrics`).
- Chart tag + app image track together via `update-policies/coder.yaml` (markers `apps:coder-chart:tag` + `apps:coder:tag` — bump both together).

## Upgrade runbook

- Version source: OCI chart tag (`oci://ghcr.io/coder/chart/coder:2.37.3`, marker `apps:coder-chart:tag`) + app image tag (`ghcr.io/coder/coder:v2.37.3`, marker `apps:coder:tag`) in `base/coder.yaml`.
- Changelog: https://github.com/coder/coder/releases (app+chart).
- Bump: let both ImagePolicy PRs land together — never one side alone.
- Migrate: snapshot `coder-db` BEFORE major bumps. Verify: dashboard OIDC login succeeds and a workspace agent connects via the `*.coder` wildcard route.
