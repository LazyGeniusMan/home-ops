# CNPG

CloudNativePG operator 1.30.0 via chart 0.29.1 (`oci://ghcr.io/cloudnative-pg/charts/cloudnative-pg`) plus a reusable `Cluster` base template (`cluster-base.yaml`, production-shaped live placeholder): 3 instances with synchronous quorum (`standbyNames: ["*"]`, number 1), `local-ssd-nvme` 20Gi, Barman S3 backup to SeaweedFS (continuous WAL gzip + daily base backup `0 0 0 * * *`, retention `30d`, prefix `s3://cnpg-backups/postgres/`), and a `*.postgres.home-ops.yansyah.my.id` wildcard `Certificate`.

Chart 0.29.1 embeds operator 1.30.0 — re-verify the chart->operator mapping in `Chart.yaml` on every bump. Failover automatic (operator promotes the most caught-up standby); switchover via `kubectl cnpg switchover <cluster>`. Restore from a new Cluster: copy the base template, replace `bootstrap.initdb` with `bootstrap.recovery.backup.name` (latest) or add `recoveryTarget.targetTime` (point-in-time; example in `cluster-base.yaml`).

## S3 contract

Endpoint `http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`. The bucket backing `s3://cnpg-backups/` must exist before the first Cluster starts. `BucketClaim`/`BucketAccess` in `configs/base/bucketclaims.yaml`. Credentials: `ExternalSecret/cnpg-s3-credentials` syncs from the COSI-minted `cnpg-backups-cosi-creds` BucketInfo JSON through the in-namespace `cnpg-cosi` SecretStore. On boto3 checksum errors set Cluster `spec.env` `AWS_REQUEST_CHECKSUM_CALCULATION` / `AWS_RESPONSE_CHECKSUM_VALIDATION` to `when_required`.

## Certificate

`Certificate/wildcard-postgres` requests `*.postgres.home-ops.yansyah.my.id` from `ClusterIssuer/letsencrypt`, stored namespace-local as `wildcard-postgres-tls`. No `DNSEndpoint` CR — the nested wildcard rides the existing `*.home-ops` wildcard A automation.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | `instances: 1` | S3 endpoint + wildcard DNS; `instances` -> 1 |
| `prd` | `instances: 3` | S3 endpoint + wildcard DNS; `instances` -> 3 |

Controllers inherit `../base` unchanged.

## Updates

`update-policies/cnpg.yaml` (marker `infra:cnpg:tag`). Take a fresh base backup before bumping. Changelogs: https://github.com/cloudnative-pg/charts/releases, https://github.com/cloudnative-pg/cloudnative-pg/releases.
