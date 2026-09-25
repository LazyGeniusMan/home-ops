# ClickHouse

Altinity clickhouse-operator 0.27.3
(`oci://registry-1.docker.io/altinity/clickhouse-operator`) plus
`ClickHouseKeeperInstallation/clickhouse-keeper` (3 replicas) and
`ClickHouseInstallation/clickhouse` (2 shards x 2 replicas), storage on
`local-ssd-nvme`, nightly S3 backups to SeaweedFS, and the
`*.clickhouse.home-ops.yansyah.my.id` wildcard `Certificate`
(`wildcard-clickhouse-tls`, own namespace, `ClusterIssuer/letsencrypt`).

Server + keeper images pinned in lockstep on the 26.8 LTS line
(`configs/base/installation-base.yaml`).

## Credentials

`ExternalSecret/clickhouse-s3-backup` syncs SeaweedFS S3 keys from the
COSI-minted BucketInfo JSON (`clickhouse-cosi-creds`) through the
in-namespace `clickhouse-cosi` SecretStore (GJSON extracts
`spec.secretS3.accessKeyID/accessSecretKey`). The CHI reads them via
`storage.xml` `from_env`. `BucketClaim`/`BucketAccess` in
`configs/base/bucketclaims.yaml`. In-cluster S3 endpoint
`http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | Keeper 1, CHI 2 shards x 1 | S3 endpoint + wildcard DNS; `replicasCount` -> 1 |
| `prd` | Keeper 3, CHI 2 shards x 2 | S3 endpoint + wildcard DNS; data volumes 100Gi per replica |

Base: 2 shards x 2 replicas, 50Gi data / 5Gi log per replica, keeper 10Gi
`Retain`, nightly backup CronJob `0 3 * * *`. `CronJob/rclone-sync-clickhouse`
uses `concurrencyPolicy: Forbid` (no scaling).

## HA / backup

Keeper quorum (3, hostname anti-affinity) backs replicated tables; create
tables replicated from day one
(`ReplicatedMergeTree('/clickhouse/tables/{shard}/events', '{replica}')`).
Keeper holds coordination state only -- no backup needed.
`CronJob/clickhouse-backup` nightly `0 3 * * *`:
`BACKUP ALL ON CLUSTER 'main' TO Disk('backups_s3', 'nightly/')`; retention
via SeaweedFS bucket lifecycle on the `nightly/` prefix. Restore from a pod
with `clickhouse-client` (`SHOW BACKUPS` / `RESTORE ALL ... FROM
Disk('backups_s3', 'nightly/<name>')`).

No `DNSEndpoint` file: external-dns watches `service` +
`gateway-httproute` sources, so records materialize from a Service
annotation or Gateway HTTPRoute.

## Telemetry / monitoring / updates

No usage-reporting keys in chart values. Metrics exporter on
(`metrics.enabled: true` + prometheus.io annotations). `ServiceMonitor` off
until `monitoring.coreos.com` CRDs land. Bumps:
`update-policies/clickhouse.yaml` (operator >=0.27.3, server/keeper
>=26.8.0; markers `infra:clickhouse:tag` + server/keeper markers) -> PR
automation. Confirm nightly `BACKUP ALL` completed before bumping; keep
server + keeper on the same line.
Changelogs: https://github.com/Altinity/clickhouse-operator/releases,
https://github.com/ClickHouse/ClickHouse/releases.
