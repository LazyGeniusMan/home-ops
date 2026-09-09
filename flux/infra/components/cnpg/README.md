# CNPG (§10.1)

CloudNativePG operator **1.30.0** (via Helm chart **0.29.0**) plus a reusable
HA `Cluster` base template: 3 instances, streaming replication with
synchronous quorum, `local-ssd-nvme` storage (the §9 default class), Barman
S3 backup to SeaweedFS, and a `*.postgres.home-ops.yansyah.my.id` wildcard
`Certificate`.

## Chart source

OCI is the upstream source of truth, verified by pull:

- `oci://ghcr.io/cloudnative-pg/charts/cloudnative-pg`, tag `0.29.0`
  (digest `sha256:209c588b902982bf283a0073db83edd422d9710a2c8a670fe57c0329abe789a4`,
  `helm template` + `helm lint` pass locally).
- Chart version and operator version differ: chart **0.29.0** embeds operator
  **1.30.0** (`appVersion: 1.30.0` in the pulled `Chart.yaml`). The
  `OCIRepository` tag therefore pins the *chart* version; operator 1.30 is
  carried by that chart.
- Update policy (`update-policies/cnpg.yaml`) follows the §7 contract
  (ImageRepository + ImagePolicy + `$imagepolicy` marker `infra:cnpg:tag`),
  but the semver floor is the **chart** line `>=0.29.0` — a `>=1.30` floor
  would never match chart tags and would deaden automation. Chart bumps stay
  on PR review, where the chart→operator mapping is re-verified before merge.

## HA / replication

- `spec.instances: 3`, one primary + two streaming standbys.
- `spec.postgresql.synchronous.standbyNames: ["*"], number: 1`: every commit
  is acknowledged by at least one standby before the primary reports success,
  so a failover never loses acknowledged writes.
- `spec.affinity.enablePodAntiAffinity: true` spreads instances across nodes.
- Failover is automatic (operator promotes the most caught-up standby);
  switchover for maintenance via `kubectl cnpg switchover <cluster>`.

## Backup / PITR runbook

`cluster-base.yaml` wires continuous archiving plus a daily base backup:

- `spec.backup.barmanObjectStore`: WAL archive (gzip) streams continuously to
  `s3://cnpg-backups/postgres/` on the SeaweedFS endpoint; `retentionPolicy:
  "30d"` keeps a month of base backups + WAL.
- `ScheduledBackup/postgres-base-backup`: full base backup daily at midnight
  (`0 0 0 * * *`, CNPG 6-field cron), owned by the Cluster object.
- Restore flows (all start from a **new** Cluster, never in place):
  - Latest state: copy the base template, replace `bootstrap.initdb` with
    `bootstrap.recovery.backup.name: <backup-object>` (a `Backup` created by
    the schedule).
  - Point-in-time: same, plus `recoveryTarget.targetTime:
    "YYYY-MM-DD HH:MM:SS.NNNNNN+00"` — WAL replay stops at that instant. A
    commented example lives in `cluster-base.yaml`.
  - Verify with `kubectl cnpg status <new-cluster>` and compare row counts
    before pointing apps at the restored cluster.

## S3 contract

- Endpoint `http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`
  (in-cluster SeaweedFS S3 FQDN; the public
  `https://s3.seaweedfs.<domain>` Gateway route is for outside-cluster
  users only) is referenced by DNS name only — SeaweedFS itself lands in
  the same wave (§12), so there is no file dependency from this
  component. The bucket backing `s3://cnpg-backups/` must exist
  **before** the first Cluster starts (Barman Cloud ≥3.16 only creates the
  bucket on the check-wal-archive path). Its COSI
  `BucketClaim`/`BucketAccess` pair lives here in
  `configs/base/bucketclaims.yaml` (also serves the `zitadel`, `coder`,
  and `ferretdb` clusters' `destinationPath` prefixes — shared bucket,
  one claim). COSI-managed replacement buckets live under
  controller-generated names — see the cosi README "Bucket cutover"
  before repointing `destinationPath`.
- Credentials: `ExternalSecret/cnpg-s3-credentials` syncs
  `ACCESS_KEY_ID`/`ACCESS_SECRET_KEY` from the COSI-minted BucketInfo JSON
  (Secret `cnpg-backups-cosi-creds`) through the in-namespace `cnpg-cosi`
  SecretStore — GJSON `property` extracts
  `spec.secretS3.accessKeyID/accessSecretKey`. Target literal keys are
  unchanged. The cross-namespace sharers (`zitadel-db`, `coder-db`,
  `ferretdb` — same bucket, own prefix) read the same keys through the
  `cosi-cnpg` ClusterSecretStore (no per-namespace claim). The Proton Pass
  `pass://…/cnpg/s3-*` entries stay seeded as rollback.
- S3-compatible quirk per upstream docs: if boto3 checksum errors appear
  (`x-amz-content-sha256`), set `spec.env` `AWS_REQUEST_CHECKSUM_CALCULATION`
  / `AWS_RESPONSE_CHECKSUM_VALIDATION` to `when_required` on the Cluster.

## Certificate + DNS

`Certificate/wildcard-postgres` requests `*.postgres.home-ops.yansyah.my.id`
from `ClusterIssuer/letsencrypt` (§9), stored as `wildcard-postgres-tls` in
the Cluster's own namespace (namespace-local TLS, same pattern as §8.1).
No `DNSEndpoint` CR is shipped: the §9 external-dns chart does not enable a
CRD source for this zone, so the nested wildcard rides on the existing
`*.home-ops` wildcard A automation (§9.4, Gateway LB target). Verify host
resolution (`dig primary.postgres.home-ops.yansyah.my.id`) before sending
client traffic over TLS.

## Telemetry-off / monitoring / updates

- Telemetry evidence: the chart `values.yaml` contains no phone-home,
  analytics, or usage-reporting knobs (checked at authoring time against the
  pulled chart); the operator exposes only a local `:8080` metrics endpoint.
- Monitoring: the in-cluster Postgres exporter is on by default upstream
  (metrics port on every instance), but nothing scrapes it — `monitoring:
  podMonitorEnabled: false` in chart values and **no** `PodMonitor`/`ServiceMonitor`
  objects are shipped until `monitoring.coreos.com` CRDs land (same §9
  deviation). Flip: set `podMonitorEnabled: true` once the monitoring stack
  exists, or apply the manual `PodMonitor` from upstream docs
  (`monitoring.md`, selector `cnpg.io/cluster: <name>`, port `metrics`).
  Note upstream deprecates `.spec.monitoring.enablePodMonitor` — prefer the
  standalone `PodMonitor`, never the in-Cluster flag.
- Operator bumps flow through `update-policies/cnpg.yaml` + PR automation;
  re-verify the chart→operator mapping on every bump (see above).

## Environments

`prd` and `stg` currently inherit `../base` unchanged (same shape
as cert-manager before per-env divergence). Per-env Cluster tuning (size,
schedule, retention) lands with the first real cluster, not here.
