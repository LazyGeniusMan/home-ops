# Dragonfly

Dragonfly operator v1.6.1 (`oci://ghcr.io/dragonflydb/dragonfly-operator/helm/dragonfly-operator`, chart == operator) plus a reusable `Dragonfly` base template (`dragonfly-base.yaml`, production-shaped live placeholder): 3 replicas (1 primary + 2 replicas) with automatic failover, tiered persistence on `local-ssd-nvme` (20Gi), hourly snapshots to SeaweedFS S3 (`0 * * * *`, master-only, prefix `s3://dragonfly-backups/dragonfly/`), and a `*.dragonfly.home-ops.yansyah.my.id` wildcard `Certificate`.

`spec.replicas: 3` counts every instance including the primary; the `<name>.<namespace>.svc.cluster.local` Service always selects the current primary. Snapshots are not pruned — age out old objects with a bucket lifecycle rule. Restore: delete the pods (or the whole object and re-apply) to load the newest snapshot; for a pinned snapshot, point a new object's `spec.snapshot.dir` at the `dump-<timestamp>.dfs` pair (never rewrite `dir` on the live object).

## Connection contract

- Host `<name>.<namespace>.svc.cluster.local`, port 6379 (RESP).
- AUTH password required: every object sets `spec.authentication.passwordFromSecret` from its ESO-synced Secret (`pass://<cluster>/dragonfly/password`, `pass://<cluster>/zitadel/cache-password`). No `spec.tlsSecretRef` — wire stays cleartext by design. The operator Deployment carries `reloader.stakater.com/auto: "true"` so password rotation rolls it.

## S3 contract

Endpoint bare host `seaweed-main-s3.seaweedfs.svc.cluster.local:8333` plus `--s3_use_https=false`. The bucket backing `s3://dragonfly-backups/` must exist before the first object starts. `BucketClaim`/`BucketAccess` in `configs/base/bucketclaims.yaml`. Credentials: `ExternalSecret/dragonfly-s3-credentials` syncs from the COSI-minted `dragonfly-backups-cosi-creds` BucketInfo JSON through the in-namespace `dragonfly-cosi` SecretStore.

## Certificate

`Certificate/wildcard-dragonfly` requests `*.dragonfly.home-ops.yansyah.my.id` from `ClusterIssuer/letsencrypt`, stored namespace-local as `wildcard-dragonfly-tls`. No `DNSEndpoint` CR — the nested wildcard rides the existing `*.home-ops` wildcard A automation.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | `replicas: 1` | S3 host + wildcard DNS; `replicas` -> 1 |
| `prd` | `replicas: 3` | S3 host + wildcard DNS; `replicas` -> 3 |

Controllers inherit `../base` unchanged.

## Updates

`update-policies/dragonfly.yaml` (>=1.6.1, marker `infra:dragonfly:tag`). Take a fresh snapshot before bumping. Changelog: https://github.com/dragonflydb/dragonfly-operator/releases.
