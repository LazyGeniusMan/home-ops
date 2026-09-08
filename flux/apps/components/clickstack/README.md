# Clickstack observability (§11.2)

HyperDX v2 (logs/traces/metrics UI) + OTel Collector, backed by a
namespace-local ClickHouse (replicated) and FerretDB on a namespace-local CNPG
Cluster (Mongo-wire, no embedded DBs in Git).

## Layout (environment-direct, apps area)

`base/` holds every manifest (`clickstack.yaml` (HyperDX app + OTel
Collector) + `ferretdb.yaml` + `oauth2-proxy.yaml` workload, plus secrets,
ClickHouseInstallation, FerretDB CNPG Cluster, wildcard certificate,
HTTPRoute); env overlays `{dev,staging,production}/` patch hostnames, vault
refs, and endpoints via `resources: [../base]`. Tenant is `apps/clickstack`
via `flux/apps/update-policies/clickstack.yaml`.

## Images (locked at authoring)

| Image | Pin | Source |
|---|---|---|
| HyperDX app `docker.hyperdx.io/hyperdx/hyperdx` | `v2.7.1` | hdx-oss-v2 chart 0.8.4 appVersion (classic repo `https://clickhouse.github.io/ClickStack-helm-charts`, `helm show chart clickstack/hdx-oss-v2`) |
| Collector `docker.hyperdx.io/hyperdx/hyperdx-otel-collector` | `v2.7.1` | same chart appVersion line (`otel.image.tag` defaults to `Chart.AppVersion`) |
| FerretDB `ghcr.io/ferretdb/ferretdb` | `2.7.0` | newest non-`latest` 2.x tag (Docker Hub tags API at authoring) |

## Why vendored, not the chart (bounded decision, 2 attempts)

`hdx-oss-v2` 0.8.4 DOES support external DBs (`mongodb.enabled=false`,
`clickhouse.enabled=false` drops both Deployments; the app takes
`hyperdx.mongoUri` and the collector takes `otel.clickhouseEndpoint`), so a
HelmRelease (attempt 1) was template-feasible. It fails on secret posture, not
templating: the chart's `hyperdx.apiKey` renders a Secret straight from values
with NO `existingSecret` knob, so the real key would land in Git — violating
the ESO-only contract every sibling follows. Attempt 2 confirmed there is no
other credential escape hatch (`defaultConnections`/`useExistingConfigSecret`
only cover connections JSON, not the API key). Hence plain Deployments
mirroring the chart's env shape (ports, probes, OPAMP wiring) with every
credential as a `secretKeyRef` to ESO-synced Secrets.

## ClickHouse backend (namespace-local CHI)

`base/clickstack-clickhouse.yaml` — namespace-local instantiation of
the §10.2 `installation-base` template (same shape, adjusted: 1 shard x 2
replicas, own S3 prefix `s3://.../clickhouse/clickstack/`, own app/otel
passwords). The component-local keeper is NOT duplicated: the CHI references
the shared `clickhouse-keeper` ensemble (infra clickhouse namespace) by name
and the operator resolves it. If keeper endpoints are not resolvable
cross-namespace, create a component-local CHK from the §10.2 base shape first.
Backups go to SeaweedFS via the `backups_s3` disk (credentials from the
`clickhouse-s3-backup` Secret — never Git). Per-table replication: create
tables with ReplicatedMergeTree + ON CLUSTER DDL (same §10.2 rule).

## FerretDB backend (Mongo-wire over CNPG)

`base/ferretdb-postgres.yaml` — namespace-local instantiation of the
§10.1 `cluster-base` template (same shape: 3 instances, sync quorum 1,
`local-ssd-nvme`, continuous WAL + daily base backup to SeaweedFS S3 under
`s3://cnpg-backups/ferretdb/`). Adjusted: dbname/owner `ferretdb`, own S3
prefix. Connection via the CNPG `-rw` Service
(`postgres://ferretdb@ferretdb-rw.clickstack.svc:5432/ferretdb`,
`sslmode=require` — CNPG serves TLS with a self-signed cert FerretDB cannot
verify, so verify-full is impossible; require still encrypts in transit, same
trade-off as the §11.1 Zitadel DSN).

Connection contract:

| Item | Value |
|---|---|
| HyperDX → FerretDB | `MONGO_URI=mongodb://ferretdb.clickstack.svc:27017/hyperdx` |
| FerretDB → Postgres | `ferretdb-rw.clickstack.svc:5432/ferretdb` (user `ferretdb`, password from `ferretdb-app-secret`; username MUST equal `initdb.owner` per upstream) |
| HyperDX → ClickHouse UI | `DEFAULT_CONNECTIONS` (`connections.json` from `clickstack-hyperdx-config`): `http://clickhouse-clickstack.clickstack.svc:8123`, user `default` (empty password; hardening follow-up adds a dedicated app user) |
| Collector → ClickHouse | `CLICKHOUSE_ENDPOINT=tcp://clickhouse-clickstack.clickstack.svc:9000?dial_timeout=10s`, user `default` (empty password; same follow-up) |
| Service naming | CHI `clickstack` → operator Service `clickhouse-clickstack` — every client above dials this host |

## Auth (locked proxy contract)

Per-instance `oauth2-proxy` (`quay.io/oauth2-proxy/oauth2-proxy:v7.6.0`)
fronts the UI; the HTTPRoute backend points at the proxy (`:4180`), which
upstreams to `http://clickstack.clickstack.svc:3000`:

| Item | Value |
|---|---|
| Issuer | `https://zitadel.home-ops.yansyah.my.id` |
| Client | `oauth2-proxy-shared` (secret via ESO, never Git) |
| Redirect | `https://clickstack.home-ops.yansyah.my.id/oauth2/callback` (covered by the registered wildcard `https://*/oauth2/callback`) |
| Cookie domain | `.home-ops.yansyah.my.id` (secure, samesite=lax) |
| Scopes | `openid profile email groups` (groups claim `groups`) |
| Gate | `allowed-group=admin` |
| Flags | `reverse-proxy=true`, `skip-provider-button=true` |

Secrets (`ExternalSecret/oauth2-proxy`): `client-secret` + `cookie-secret`
(32 random bytes) from Proton Pass
(`pass://acme-prd-bdo1-talos-apps-01/clickstack/oauth2-proxy-*`). Seed the
vault entries with pass-cli. The Zitadel `clickstack` client is already declared
— reference only.

## Routing

`base/clickstack-httproute.yaml` — HTTPRoute on the shared §8.1 Gateway
(`main`, cross-namespace parentRef, `https` section): hostname
`clickstack.home-ops.yansyah.my.id`, `/` → `oauth2-proxy:4180`. TLS terminates
at the Gateway via the in-namespace wildcard `Certificate`
(`wildcard-certificate.yaml`, same duplicate pattern as §8.1 — cert-manager
Secrets are namespace-local).

## Telemetry-off / monitoring / updates

- Telemetry evidence: the pulled hdx-oss-v2 0.8.4 chart exposes exactly one
  usage-reporting knob (`hyperdx.usageStatsEnabled`, defaults true); the
  vendored app Deployment sets `USAGE_STATS_ENABLED=false` explicitly, no
  Sentry/Segment/Mixpanel envs are set anywhere, and the app's
  `OTEL_EXPORTER_OTLP_ENDPOINT` points at the in-namespace collector
  (`clickstack-otel-collector:4318`) — nothing leaves the cluster.
- Unguarded monitors OFF: no `ServiceMonitor` objects are shipped until
  `monitoring.coreos.com` CRDs land (same §9 deviation). Flip: add
  `ServiceMonitor`s for the collector metrics port (8888) and the app once the
  monitoring stack exists.
- Updates flow through `flux/apps/update-policies/clickstack.yaml`
  (ImageRepository + ImagePolicy, `$imagepolicy` markers on all four images;
  the HyperDX floors `>=2.7.1` track the appVersion line, FerretDB `>=2.7.0`).

## Environments

`production` and `staging` currently inherit `../base` unchanged (same shape
as cert-manager before per-env divergence). Per-env tuning (replica count,
schedule knobs) lands with the first real divergence, not here.
