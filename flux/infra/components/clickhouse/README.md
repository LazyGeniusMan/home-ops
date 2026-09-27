# ClickHouse

Altinity clickhouse-operator 0.27.3 (`oci://registry-1.docker.io/altinity/clickhouse-operator`) plus `ClickHouseKeeperInstallation/clickhouse-keeper` (3 replicas) and `ClickHouseInstallation/clickhouse` (2 shards x 2 replicas), storage on `local-ssd-nvme`, nightly S3 backups to SeaweedFS, and the `*.clickhouse.home-ops.yansyah.my.id` wildcard `Certificate` (`wildcard-clickhouse-tls`).

Server + keeper images stay in lockstep on the 26.8 LTS line (`configs/base/installation-base.yaml`).

## Credentials

`ExternalSecret/clickhouse-s3-backup` syncs SeaweedFS S3 keys from the COSI-minted `clickhouse-cosi-creds` BucketInfo JSON through the in-namespace `clickhouse-cosi` SecretStore. `BucketClaim`/`BucketAccess` in `configs/base/bucketclaims.yaml`. S3 endpoint `http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`.

Client auth: dedicated `otel` CHI user — `ExternalSecret/clickhouse-auth` syncs `otel-password` from `pass://<cluster>/clickhouse/otel-password` (same value as the `otel-clickhouse` mirror in the otel-collectors namespace). Consumers: otel-gateway exporter (`username: otel`) and the nightly backup CronJob (`--user=otel`). The operator-locked `default` user keeps its empty password for localhost/distributed-query only.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | Keeper 1, CHI 2 shards x 1 | S3 endpoint + wildcard DNS; `replicasCount` -> 1 |
| `prd` | Keeper 3, CHI 2 shards x 2 | S3 endpoint + wildcard DNS; data volumes 100Gi per replica |

Base: 2 shards x 2 replicas, 50Gi data / 5Gi log per replica, keeper 10Gi `Retain`, nightly backup CronJob `0 3 * * *` (`BACKUP ALL ON CLUSTER 'main' TO Disk('backups_s3', 'nightly/')`, retention via bucket lifecycle; restore with `clickhouse-client` `RESTORE ALL ... FROM Disk('backups_s3', 'nightly/<name>')`). Tables must be created replicated from day one (`ReplicatedMergeTree('/clickhouse/tables/{shard}/events', '{replica}')`). No `DNSEndpoint` file — external-dns watches `service` + `gateway-httproute` sources.

## Updates

`update-policies/clickhouse.yaml` (operator >=0.27.3, server/keeper >=26.8.0; markers `infra:clickhouse:tag` + server/keeper markers). Confirm nightly `BACKUP ALL` completed before bumping; keep server + keeper on the same line. Changelogs: https://github.com/Altinity/clickhouse-operator/releases, https://github.com/ClickHouse/ClickHouse/releases.
