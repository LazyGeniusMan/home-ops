# ClickHouse (§10.2)

Altinity clickhouse-operator 0.27.3 plus a reusable HA base:
`ClickHouseKeeperInstallation/clickhouse-keeper` (3 replicas) and
`ClickHouseInstallation/clickhouse` (2 shards x 2 replicas), storage on
`local-ssd-nvme`, nightly S3 backups to SeaweedFS, and the
`*.clickhouse.home-ops.yansyah.my.id` wildcard `Certificate`
(`wildcard-clickhouse-tls`, own-namespace, `ClusterIssuer/letsencrypt`).

## Chart source

OCI-first (`oci://registry-1.docker.io/altinity/clickhouse-operator`, tag
`0.27.3` verified present on Docker Hub) so chart bumps flow through the §7
update-policy contract (`update-policies/clickhouse.yaml` → PR automation via
the `infra:clickhouse:tag` marker). Fallback, with justification: the official
classic repo `https://helm.altinity.com` (chart 0.27.3 confirmed in its index)
— switch the `OCIRepository` to a `HelmRepository` only if the OCI artifact
fails to resolve. OCI probes during authoring: Docker Hub tag exists (image
line shares the version, artifact type ambiguous), ghcr.io denied, no Altinity
OCI registry resolves — hence OCI primary with a documented fallback.

## Credentials

`ExternalSecret/clickhouse-s3-backup` syncs the SeaweedFS S3 keys from the
COSI-minted BucketInfo JSON (Secret `clickhouse-cosi-creds`) through the
in-namespace `clickhouse-cosi` SecretStore — GJSON `property` extracts
`spec.secretS3.accessKeyID/accessSecretKey` (keys need read/write on the
`clickhouse/` bucket prefix). The CHI reads them via `storage.xml`
`from_env` — no credential value is committed anywhere in this component.
Decommissioned vault paths (`pass://…/clickhouse/s3-*`) are deleted; no
rollback entries are kept. Create the bucket prefix once via the SeaweedFS
S3 API. In-cluster
`storage.xml` endpoints use the FQDN
`http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`. Its COSI
`BucketClaim`/`BucketAccess` pair lives here in
`configs/base/bucketclaims.yaml` (one pair per live bucket — see the cosi
README).

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | Keeper 1, CHI 1 shard x 1 replica (single-instance, no quorum) | S3 endpoint + wildcard DNS; `replicasCount` → 1 on keeper + CHI |
| `prd` | Keeper 3, CHI 2 shards x 2 replicas (recommended production) | S3 endpoint + wildcard DNS; `replicasCount` → 3 (keeper) / 2 (CHI); data volumes 100Gi per replica |

- Base: 2 shards x 2 replicas, 50Gi data / 5Gi log per replica, nightly backup
  CronJob enabled.
- `prd`: data volumes grow to 100Gi per replica (patch on
  `ClickHouseInstallation/clickhouse`).
- `dev` carries its own S3 endpoint + wildcard DNS patches; controllers
  in both envs inherit `../base` unchanged.
- Rclone sync (`CronJob/rclone-sync-clickhouse`): 1 per instance/schedule,
  `concurrencyPolicy: Forbid` — no scaling; overlapping runs must never
  fight over the Proton destination.

Upstream reference (read-only): `/tmp/home-ops-docs/altinity-clickhouse-operator-docs`
(+ `/tmp/home-ops-docs/clickhouse-docs` — server `ReplicatedMergeTree` /
keeper table semantics).

## HA

Keeper quorum (3 replicas, hostname anti-affinity, `Retain` data volumes)
backs replicated tables; each shard keeps 2 replicas so one replica loss
neither blocks writes nor loses quorum. Create tables replicated from day one:

```sql
CREATE TABLE events ON CLUSTER 'main' (...) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/events', '{replica}');
```

Keeper holds coordination state only — it needs no backup; a fresh ensemble
re-converges and CHI replicas re-register.

## Backup / restore runbook

`CronJob/clickhouse-backup` runs nightly (`0 3 * * *`):
`BACKUP ALL ON CLUSTER 'main' TO Disk('backups_s3', 'nightly/')`. Every BACKUP
is an immutable snapshot; enforce retention with SeaweedFS bucket lifecycle on
the `nightly/` prefix (e.g. expire after 30d).

Restore (run from any pod with `clickhouse-client`, e.g. a debug pod against
host `clickhouse`):

```sql
-- list available snapshots
SHOW BACKUPS FROM Disk('backups_s3', 'nightly/');
-- full restore (stops writes first; drops conflicting tables)
RESTORE ALL ON CLUSTER 'main' FROM Disk('backups_s3', 'nightly/<name>');
```

Single-table restore: `RESTORE TABLE db.tbl ON CLUSTER 'main' FROM
Disk('backups_s3', 'nightly/<name>')`. After restore, run
`SYSTEM SYNC REPLICA ON CLUSTER 'main' db.tbl` and verify row counts per
shard before re-enabling writers.

## Upgrade runbook

- Version source: the `OCIRepository` tag in
  `controllers/base/clickhouse-operator.yaml` (operator 0.27.3) plus the
  server + keeper image pins on the ClickHouseInstallation pod templates
  (26.8 LTS lockstep — see the header in
  `configs/base/installation-base.yaml`).
- Changelog (operator):
  https://github.com/Altinity/clickhouse-operator/releases. Changelog
  (server/keeper): https://github.com/ClickHouse/ClickHouse/releases.
- Bump: let the ImagePolicy PRs land (markers `infra:clickhouse:tag`
  plus the server/keeper markers, `update-policies/clickhouse.yaml`).
  Read the server changelog before taking a new LTS (25.9 went EOL
  2025-12; the 26.x jump crosses breaking changes) and keep server +
  keeper on the same line.
- Migrate: confirm the nightly `BACKUP ALL` completed BEFORE the bump
  (see the Backup / restore runbook above). Verify: keeper quorum
  `Ready`, CHI `status` shows all replicas, and row counts match per
  shard before re-enabling writers.

## DNS

No `DNSEndpoint` file (bounded decision, same as the CNPG approach):
external-dns already watches `service` + `gateway-httproute` sources, so the
`*.clickhouse...` records materialize from a `Service` annotation or a
Gateway `HTTPRoute` created alongside the consumer (§10.x), with no file
overlap here. If a static record is ever needed, add it in the consumer, not
in this component.

## Telemetry-off / monitoring / updates

- No usage-reporting/telemetry keys in chart values (verified: non-comment
  values lines matching `telemetry|usageReporting|phoneHome|analytics` are
  empty), so there is nothing to switch off. Re-verify with:
  `grep -rin 'telemetry\|phonehome\|analytics\|usagereport' <chart> | grep -v '^.*#'`.
- Metrics exporter on (`metrics.enabled: true` + `prometheus.io` annotations
  on keeper pods — safe without a stack).
- `ServiceMonitor` (both `serviceMonitor.enabled` and
  `serviceMonitor.keeperMetrics.enabled`) stays disabled until
  `monitoring.coreos.com` CRDs land. Flip note: set both to `true` once
  kube-prometheus lands; keeper scraping additionally needs the CHK to expose
  a `prometheus` port via a `serviceTemplate` (a template REPLACES the default
  port list, so re-declare `zk`/`raft` there) — see the chart's
  `servicemonitor-keeper.yaml` head comment.
- Operator + chart bumps: `update-policies/clickhouse.yaml` → PR automation.
