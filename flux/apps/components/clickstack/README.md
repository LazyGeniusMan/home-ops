# Clickstack observability

HyperDX v2 (logs/traces/metrics UI) + OTel Collector, backed by a namespace-local ClickHouse (1 shard x 2
replicas) and FerretDB on a namespace-local CNPG Cluster (Mongo-wire, no embedded DBs in Git).

## Layout

`base/` holds every manifest (`clickstack.yaml` app + collector Deployments, `ferretdb.yaml` Deployment,
`oauth2-proxy.yaml` OCIRepository + HelmRelease, secrets, ClickHouseInstallation, FerretDB CNPG Cluster, wildcard certificate,
HTTPRoute); env overlays `{dev,prd}/` patch hostnames, vault refs, and endpoints via `resources: [../base]`.

## Images

| Image | Pin | Source |
|---|---|---|
| HyperDX app `docker.hyperdx.io/hyperdx/hyperdx` | `v2.7.1` | hdx-oss-v2 chart 0.8.4 appVersion |
| Collector `docker.hyperdx.io/hyperdx/hyperdx-otel-collector` | `v2.7.1` | same chart appVersion (`otel.image.tag` defaults to `Chart.AppVersion`) |
| FerretDB `ghcr.io/ferretdb/ferretdb` | `2.7.0` | newest non-`latest` 2.x tag |

## Backends

- ClickHouse (`base/clickstack-clickhouse.yaml`): namespace-local CHI, 1 shard x 2 replicas, `local-ssd-nvme`, S3
  backups to `clickhouse/clickstack/`, no users (operator `default`, empty password). Keeper reuses the shared infra `clickhouse-keeper` ensemble. Tables use ReplicatedMergeTree + ON CLUSTER DDL.
- FerretDB (`base/ferretdb-postgres.yaml` + `base/ferretdb.yaml`): CNPG Cluster (3 instances, sync quorum 1, WAL + daily
  base backup to `s3://cnpg-backups/ferretdb/`, dbname/owner `ferretdb`) + stateless Mongo-wire proxy. `sslmode=require` (CNPG self-signed cert; require still encrypts in transit).

| Item | Value |
|---|---|
| HyperDX → FerretDB | `MONGO_URI=mongodb://ferretdb.clickstack.svc:27017/hyperdx` |
| FerretDB → Postgres | `ferretdb-rw.clickstack.svc:5432/ferretdb` (user `ferretdb`, password from `ferretdb-app-secret`; username MUST equal `initdb.owner`) |
| HyperDX → ClickHouse | `DEFAULT_CONNECTIONS` (`connections.json`): `http://clickhouse-clickstack.clickstack.svc:8123`, user `default` |
| Collector → ClickHouse | `CLICKHOUSE_ENDPOINT=tcp://clickhouse-clickstack.clickstack.svc:9000?dial_timeout=10s`, user `default` |

## Auth

Per-instance `oauth2-proxy` (official OCI chart 10.7.0, app v7.15.4) fronts the UI; the HTTPRoute backend points at
the proxy (`:4180`), which upstreams to `http://clickstack.clickstack.svc:3000`:

| Item | Value |
|---|---|
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Client | `clickstack` (server-generated, via ESO — never Git) |
| Redirect | `https://clickstack.home-ops.yansyah.my.id/oauth2/callback` |
| Cookie domain | `.home-ops.yansyah.my.id` (secure, samesite=lax) |
| Scopes | `openid profile email groups` |
| Gate | `allowed-group=clickstack-admin` |
| Flags | `reverse-proxy=true`, `skip-provider-button=true` |

`ExternalSecret/oauth2-proxy-oidc` syncs `client-id` + `client-secret` from the `clickstack-sso-outputs` Secret via
the in-cluster `clickstack-k8s` SecretStore (no pass:// seeding for OIDC creds); `oauth2-proxy-cookie` syncs the cookie secret
(32 random bytes) from Proton Pass (`pass://acme-prd-bdo1-talos-apps-01/clickstack/oauth2-proxy-cookie-secret`). The
`clickstack` Zitadel client is owned by this app's `clickstack-sso` Terraform CR (org_id + admin ID mirror from the FirstInstance handoff via ESO; no `org_id` literal in git).

## Routing + TLS

HTTPRoute on the shared `main` Gateway (`https` section, cross-namespace parentRef): `clickstack.home-ops.yansyah.my.id`
→ `oauth2-proxy:4180`. TLS terminates at the Gateway via the in-namespace wildcard `Certificate` (cert-manager Secrets are namespace-local).

## Telemetry / monitoring / updates

- Telemetry off: `USAGE_STATS_ENABLED=false` (chart knob defaults true); app OTLP points at the in-namespace collector — nothing leaves the cluster. `ServiceMonitor: off` (no monitoring CRDs).
- Updates via `flux/apps/update-policies/clickstack.yaml` (ImageRepository + ImagePolicy + `$imagepolicy` markers; proxy markers shared with hubble-ui + flux-operator-ui — bump together).

## Upgrade runbook

- Version source: `base/clickstack.yaml` (HyperDX app + collector), `base/ferretdb.yaml` (FerretDB), `base/oauth2-proxy.yaml` (chart + app).
- Changelog: HyperDX https://github.com/hyperdxio/hyperdx/releases · FerretDB https://github.com/FerretDB/FerretDB/releases · proxy chart https://github.com/oauth2-proxy/manifests/releases · proxy image https://github.com/oauth2-proxy/oauth2-proxy/releases.
- Bump: let the ImagePolicy PRs land; move the proxy pins together with hubble-ui + flux-operator-ui. The CHI server marker tracks the infra clickhouse line — check it before a server-coupled bump.
- Migrate: snapshot the `ferretdb` CNPG cluster + confirm a ClickHouse `BACKUP ALL` completed BEFORE major bumps. Verify: the logs/traces UI loads through the proxy and the collector still receives OTLP.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | app 1, collector 1, oauth2-proxy 1, ferretdb 1, `ferretdb` Cluster 1, CHI 1x1 | vault refs, hostnames, endpoints + counts → 1 |
| `prd` | app 2, collector 2, oauth2-proxy 1, ferretdb 1, `ferretdb` Cluster 3, CHI 1x2 | vault refs, hostnames, endpoints + production counts |

oauth2-proxy and FerretDB stay singletons (1) in every env — never scale them. Rclone sync (`ferretdb` + `clickstack`
legs): 1 per instance/schedule, `concurrencyPolicy: Forbid` — no scaling.

Upstream reference (read-only): `/tmp/home-ops-docs/clickstack-helm-charts-docs`.
