# Dragonfly

Dragonfly operator v1.6.1
(`oci://ghcr.io/dragonflydb/dragonfly-operator/helm/dragonfly-operator`,
chart == operator so no chart->operator mapping to re-verify) plus a
reusable `Dragonfly` base template: 3 replicas (1 primary + 2 replicas)
with automatic failover, tiered persistence on `local-ssd-nvme` (20Gi),
hourly snapshots to SeaweedFS S3 (`0 * * * *`, master-only, prefix
`s3://dragonfly-backups/dragonfly/`), and a
`*.dragonfly.home-ops.yansyah.my.id` wildcard `Certificate`.

## HA

`spec.replicas: 3` counts every instance including the primary. The
operator keeps one primary serving; the `<name>.<namespace>.svc.cluster.local`
Service always selects the current primary. Preferred pod anti-affinity
(`kubernetes.io/hostname`) spreads replicas across nodes.

## Connection contract

- Host: `<name>.<namespace>.svc.cluster.local` (e.g.
  `zitadel-cache.<namespace>.svc.cluster.local`).
- Port 6379 (RESP; `redis-cli -h <host>` works).
- No password/TLS by default: the base ships neither
  `spec.authentication.passwordFromSecret` nor `spec.tlsSecretRef`. The
  `wildcard-dragonfly-tls` Secret is the `tlsSecretRef` candidate (must
  carry `tls.crt`/`tls.key` in the Dragonfly namespace).

## Backup / restore

`spec.snapshot`: `dir: s3://dragonfly-backups/dragonfly/`,
`cron: "0 * * * *"` (hourly), `enableOnMasterOnly: true`. Dragonfly
auto-loads the latest snapshot from `dir` on startup. Snapshots are not
pruned -- age out old objects with a bucket lifecycle rule or sweep
manually. Restore: delete the pods (or the whole Dragonfly object and
re-apply) to load the newest snapshot; for a pinned snapshot, place the
`dump-<timestamp>.dfs` (+ `-summary.dfs`) pair under a prefix and point a
new Dragonfly object's `spec.snapshot.dir` at it (never rewrite `dir` on
the live object).

## S3 contract

Endpoint bare host `seaweed-main-s3.seaweedfs.svc.cluster.local:8333`
(`--s3_endpoint` takes no scheme; the public Gateway route is for
outside-cluster users only). The bucket backing `s3://dragonfly-backups/`
must exist before the first Dragonfly object starts. `BucketClaim` /
`BucketAccess` in `configs/base/bucketclaims.yaml`. Credentials:
`ExternalSecret/dragonfly-s3-credentials` syncs `ACCESS_KEY_ID` /
`SECRET_ACCESS_KEY` from the COSI-minted BucketInfo JSON
(`dragonfly-backups-cosi-creds`) through the in-namespace `dragonfly-cosi`
SecretStore. `--s3_use_https=false` rides alongside (internal S3 is plain
HTTP) and `AWS_REGION` satisfies the SDK credential chain.

## Certificate / DNS

`Certificate/wildcard-dragonfly` requests
`*.dragonfly.home-ops.yansyah.my.id` from `ClusterIssuer/letsencrypt`,
stored as `wildcard-dragonfly-tls` namespace-local. No `DNSEndpoint` CR:
the nested wildcard rides the existing `*.home-ops` wildcard A automation.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | `replicas: 1` | S3 host + wildcard DNS; `replicas` -> 1 |
| `prd` | `replicas: 3` | S3 host + wildcard DNS; `replicas` -> 3 |

Controllers inherit `../base` unchanged.
`CronJob/rclone-sync-dragonfly-backups` uses `concurrencyPolicy: Forbid`.

## Telemetry / monitoring / updates

No phone-home knobs in chart values (local metrics endpoint `:8080` behind
the `:8443` kube-rbac-proxy sidecar). No `ServiceMonitor` shipped;
`serviceMonitor.enabled`/`grafanaDashboard.enabled` stay `false` until
`monitoring.coreos.com` CRDs land. Bumps: `update-policies/dragonfly.yaml`
(>=1.6.1, marker `infra:dragonfly:tag`) -> PR automation. Take a fresh
snapshot before bumping.
Changelog: https://github.com/dragonflydb/dragonfly-operator/releases.
