# CNPG

CloudNativePG operator 1.30.0 via chart 0.29.1
(`oci://ghcr.io/cloudnative-pg/charts/cloudnative-pg`) plus a reusable
`Cluster` base template: 3 instances, streaming replication with
synchronous quorum (`standbyNames: ["*"]`, number 1), `local-ssd-nvme`
storage (20Gi), Barman S3 backup to SeaweedFS (continuous WAL gzip + daily
base backup `0 0 0 * * *`, retention `30d`, prefix
`s3://cnpg-backups/postgres/`), and a `*.postgres.home-ops.yansyah.my.id`
wildcard `Certificate`.

Chart 0.29.1 embeds operator 1.30.0 (`appVersion: 1.30.0`) -- the
`OCIRepository` tag pins the chart; the policy floor `>=0.29.1` tracks the
chart line. Re-verify the chart->operator mapping in `Chart.yaml` on every
bump.

## HA

`spec.instances: 3`, one primary + two streaming standbys;
`enablePodAntiAffinity: true`. Failover automatic (operator promotes the
most caught-up standby); switchover via `kubectl cnpg switchover <cluster>`.

## Backup / PITR

`spec.backup.barmanObjectStore` streams WAL (gzip) to
`s3://cnpg-backups/postgres/`; `ScheduledBackup/postgres-base-backup` runs
daily at midnight (CNPG 6-field cron). Restore starts from a new Cluster:
copy the base template, replace `bootstrap.initdb` with
`bootstrap.recovery.backup.name: <backup>` (latest) or add
`recoveryTarget.targetTime` (point-in-time; commented example in
`cluster-base.yaml`).

## S3 contract

Endpoint `http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`
(in-cluster SeaweedFS S3; the public Gateway route is for outside-cluster
users only). The bucket backing `s3://cnpg-backups/` must exist before the
first Cluster starts. `BucketClaim`/`BucketAccess` in
`configs/base/bucketclaims.yaml`. Credentials:
`ExternalSecret/cnpg-s3-credentials` syncs `ACCESS_KEY_ID`/`ACCESS_SECRET_KEY`
from the COSI-minted BucketInfo JSON (`cnpg-backups-cosi-creds`) through the
in-namespace `cnpg-cosi` SecretStore. S3-compatible quirk: on boto3 checksum
errors set Cluster `spec.env` `AWS_REQUEST_CHECKSUM_CALCULATION` /
`AWS_RESPONSE_CHECKSUM_VALIDATION` to `when_required`.

## Certificate / DNS

`Certificate/wildcard-postgres` requests `*.postgres.home-ops.yansyah.my.id`
from `ClusterIssuer/letsencrypt`, stored as `wildcard-postgres-tls`
namespace-local. No `DNSEndpoint` CR: the nested wildcard rides the existing
`*.home-ops` wildcard A automation.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | `instances: 1` | S3 endpoint + wildcard DNS; `instances` -> 1 |
| `prd` | `instances: 3` | S3 endpoint + wildcard DNS; `instances` -> 3 |

Controllers inherit `../base` unchanged.
`CronJob/rclone-sync-cnpg-backups` uses `concurrencyPolicy: Forbid`.

## Telemetry / monitoring / updates

No phone-home knobs in chart values (local `:8080` metrics only). Postgres
exporter on by default upstream but unscraped; `monitoring.podMonitorEnabled:
false` and no PodMonitor/ServiceMonitor until `monitoring.coreos.com` CRDs
land (prefer the standalone PodMonitor; upstream deprecates
`.spec.monitoring.enablePodMonitor`). Bumps:
`update-policies/cnpg.yaml` (marker `infra:cnpg:tag`) -> PR automation.
Take a fresh base backup before bumping.
Changelogs: https://github.com/cloudnative-pg/charts/releases,
https://github.com/cloudnative-pg/cloudnative-pg/releases.
