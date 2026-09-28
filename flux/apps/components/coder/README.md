# Coder

Self-hosted remote dev environments at `https://coder.home-ops.yansyah.my.id`, app `v2.37.3` via chart `oci://ghcr.io/coder/chart/coder` `2.37.3`
(chart↔app lockstep; no digest pin — upstream publishes the chart OCI artifact unsigned).

Coderd requests `500m/512Mi` feed the HPA denominator; the `2000m` CPU limit is burst headroom for provisioner spikes. No custom workspace template.

## Layout

`base/` holds every manifest (`coder.yaml`, secrets, CNPG Cluster, wildcard certificates, HTTPRoutes); `{dev,prd}/` patch hostnames, vault refs, and chart values.

## OIDC (direct — no oauth2-proxy)

SSO owned by this app: the `coder-sso` Terraform CR owns the `coder` Zitadel project + `coder-admin` / `coder-user` roles + OIDC client + users from `var.user_emails`.

| Item | Value |
|---|---|
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Redirect | `https://coder.home-ops.yansyah.my.id/*` |
| Scopes / groups | `openid,profile,email,groups` / `coder-admin,coder-user` |
| Identity map | `admin@…` → `admin`/`coder-admin`; `git@…` → `git`/`coder-user` |

First OIDC login claims instance ownership — perform it as `admin@home-ops.yansyah.my.id` first. Base ships `CODER_DISABLE_PASSWORD_AUTH="false"` (kill-switch, `base/coder.yaml` `/env/10`, never patched per-env); flip to `"true"` after OIDC login succeeds in the env.

## Routing

HTTPRoutes on shared `main` Gateway: `coder` (`coder.home-ops.yansyah.my.id`) and `coder-workspaces` (`*.coder.home-ops.yansyah.my.id`) → `coder:80`, plus HTTP→HTTPS 301s.

## TLS + DNS

Two in-namespace Certificates: `coder-root` (`coder.home-ops.yansyah.my.id`) and `coder-wildcard` (`*.coder.home-ops.yansyah.my.id`), both via `ClusterIssuer/letsencrypt` DNS-01.

## Database

`base/coder-db.yaml` — `coder` CNPG Cluster (3 instances, sync quorum 1, WAL + daily base backup to `s3://cnpg-backups/coder/`). `CODER_PG_CONNECTION_URL` reads `coder-db-credentials` (`sslmode=require`).

## Credentials

`coder-oidc` (stored Terraform outputs via `coder-k8s`), `coder-db-credentials` + `coder-db-app-secret` (single `.../coder/db-password` vault source), `cnpg-s3-credentials` (COSI-minted, claim `coder-db`), `cloudflare-api-token` (DNS-01). Matrix notifier (`matrix-notify`) composes from the matrix kept Secret via `coder-matrix` (zero vault seeding; known 204 gap: see matrix `NOTIFICATIONS.md`).

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | coderd 1, `coder-db` Cluster 1 | vault refs, hostnames, chart values |
| `prd` | coderd 2, `coder-db` Cluster 3 | vault refs, hostnames, chart values |

## Updates

Policy `update-policies/coder.yaml` (markers `apps:coder-chart:tag` + `apps:coder:tag` — bump together). Changelog: [coder](https://github.com/coder/coder/releases). Snapshot `coder-db` before major bumps.
