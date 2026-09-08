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

`ExternalSecret/clickhouse-s3-backup` syncs the SeaweedFS S3 keys from Proton
Pass (`pass://acme-prd-bdo1-talos-apps-01/clickhouse/s3-access-key-id` and
`.../s3-secret-access-key`; keys need read/write on the `clickhouse/` bucket
prefix). The CHI reads them via `storage.xml` `from_env` — no credential value
is committed anywhere in this component. Seed the vault entries with pass-cli
and create the bucket prefix once via the SeaweedFS S3 API.

## Environments

- Base: 2 shards x 2 replicas, 50Gi data / 5Gi log per replica, nightly backup
  CronJob enabled.
- `prd`: data volumes grow to 100Gi per replica (patch on
  `ClickHouseInstallation/clickhouse`).
- `stg`: single shard / single replica, 20Gi data, backup CronJob
  suspended (patch on `CronJob/clickhouse-backup`).

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
