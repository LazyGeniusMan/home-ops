# Dragonfly (§10.3)

Dragonfly operator **1.6.1** plus a reusable HA `Dragonfly` base template: 3
replicas (1 primary + 2 replicas) with automatic failover, tiered persistence
on `local-ssd-nvme` (the §9 default class), hourly snapshots to SeaweedFS S3,
and a `*.dragonfly.home-ops.yansyah.my.id` wildcard `Certificate`.

## Chart source

OCI is the upstream source of truth, verified by pull:

- `oci://ghcr.io/dragonflydb/dragonfly-operator/helm/dragonfly-operator`, tag
  `v1.6.1` (digest
  `sha256:a2e9f431f46b0dfb4aee426b70efb4394f970525516ea3f51c8d60a345bbc260`,
  `helm template` + `helm lint` pass locally).
- Unlike §10.1 (cnpg), chart version and operator version agree here: chart
  **v1.6.1** embeds operator **v1.6.1** (`appVersion: v1.6.1` in the pulled
  `Chart.yaml`). No chart→operator mapping to re-verify on bumps, but the
  `update-policies/dragonfly.yaml` floor is still `>=1.6.1` per the §7
  contract (ImageRepository + ImagePolicy + `$imagepolicy` marker
  `infra:dragonfly:tag`).
- No OCI fallback was needed: the operator's own release workflow (`ci.yml`
  @ v1.6.1) pushes the chart to exactly this GHCR path on every release.
- Local reference: the d2 docs carry no Dragonfly component, so the bounded
  source was the operator docs (`dragonfly-operator-docs`, upstream
  [documentation](https://github.com/dragonflydb/documentation) branch
  `main`) plus the operator repo @ v1.6.1 (CRD schema, `resources.go`
  snapshot/tiering logic, release assets).

## HA / replication

- `spec.replicas: 3`: 1 primary + 2 replicas (operator semantics — `replicas`
  counts every instance including the primary).
- The operator keeps one primary serving and reconfigures replication as pods
  churn; the `<name>.<namespace>.svc.cluster.local` Service always selects
  the current primary, so clients never track pod identity.
- Preferred pod anti-affinity (`kubernetes.io/hostname`) spreads replicas
  across nodes. Failover is automatic; check `kubectl describe
  dragonfly <name>` (`status.phase: ready`) after a primary loss.

## Connection contract (§11 Zitadel)

Zitadel consumes this base as its Redis-compatible cache. The contract a
consumer needs (also stated in `dragonfly-base.yaml`):

- Host: `<name>.<namespace>.svc.cluster.local`
  (e.g. `zitadel-cache.<namespace>.svc.cluster.local` once §11 names it).
- Port: **6379** (Dragonfly speaks RESP; `redis-cli -h <host>` works).
- No password/TLS by default: the base ships neither
  `spec.authentication.passwordFromSecret` nor `spec.tlsSecretRef`, so enable
  them on the namespace-local copy if the consumer requires auth/TLS (the
  `wildcard-dragonfly-tls` Secret from the wildcard `Certificate` in this
  directory is the `tlsSecretRef` candidate; per upstream docs the Secret must
  carry `tls.crt`/`tls.key` and live in the Dragonfly namespace).

## Backup / restore runbook

`dragonfly-base.yaml` snapshots hourly to SeaweedFS S3:

- `spec.snapshot`: `dir: s3://dragonfly-backups/dragonfly/`,
  `cron: "0 * * * *"` (hourly), `enableOnMasterOnly: true` so only the
  primary writes snapshots. Filenames default to `dump-{timestamp}`
  (`--dbfilename`). Dragonfly auto-loads the latest snapshot from `dir` on
  startup, so a pod restart or reschedule restores without intervention.
- Snapshots are NOT pruned by Dragonfly — timestamped dumps accumulate in the
  bucket. Age out old objects with a bucket lifecycle rule once SeaweedFS S3
  (§12) exposes one, or sweep manually.
- Restore flows (Dragonfly has no PITR/WAL replay — restore means loading a
  chosen snapshot file):
  - Latest state: delete the pods (or the whole Dragonfly object and
    re-apply); the operator recreates them and Dragonfly loads the newest
    snapshot from `s3://dragonfly-backups/dragonfly/` automatically. Verify
    key counts before pointing apps back.
  - Point-in-time / pinned snapshot: download the wanted
    `dump-<timestamp>.dfs` (+ `-summary.dfs`) pair from the bucket, place both
    under a prefix (e.g. `s3://dragonfly-backups/dragonfly/restore/`), point a
    **new** Dragonfly object's `spec.snapshot.dir` at that prefix, apply, and
    verify before cutting traffic over. Never rewrite `dir` on the live
    object — start a fresh instance for the restore.
  - Disaster (bucket empty): the instance boots empty; repopulate from the
    source of truth (Zitadel rebuilds cache from Postgres).

## S3 contract

- Endpoint `seaweed-main-s3.seaweedfs.svc.cluster.local:8333`
  (in-cluster SeaweedFS S3 FQDN, bare host — `--s3_endpoint` takes no
  scheme; the public `https://s3.seaweedfs.<domain>` Gateway route is for
  outside-cluster users only) is referenced by DNS name only — SeaweedFS
  itself lands in the same wave (§12), so there is no file dependency from
  this component. The bucket backing `s3://dragonfly-backups/` must exist
  **before** the first Dragonfly object starts (Dragonfly never creates
  buckets). Its COSI `BucketClaim`/`BucketAccess` pair lives here in
  `configs/base/bucketclaims.yaml` (also serves the `zitadel-cache`
  snapshot prefix — shared bucket, one claim).
- Credentials: `ExternalSecret/dragonfly-s3-credentials` syncs
  `ACCESS_KEY_ID`/`SECRET_ACCESS_KEY` from Proton Pass
  (`pass://acme-prd-bdo1-talos-apps-01/dragonfly/s3-access-key-id`,
  `pass://acme-prd-bdo1-talos-apps-01/dragonfly/s3-secret-access-key`). Seed
  the vault entries with pass-cli. The Dragonfly object consumes them as a
  same-namespace secret via `spec.env` — copy the `ExternalSecret` into each
  namespace that hosts a Dragonfly instance.
- S3-compatible quirk: `--s3_endpoint` overrides the AWS endpoint and the
  `AWS_REGION` env satisfies the SDK credential chain for non-AWS backends
  (both set on the base object per upstream backup docs).
  `--s3_use_https=false` rides alongside (internal S3 is plain HTTP;
  `s3_use_https` defaults true).

## Certificate + DNS

`Certificate/wildcard-dragonfly` requests `*.dragonfly.home-ops.yansyah.my.id`
from `ClusterIssuer/letsencrypt` (§9), stored as `wildcard-dragonfly-tls` in
the Dragonfly namespace (namespace-local TLS, same pattern as §8.1/§10.1).
No `DNSEndpoint` CR is shipped: the §9 external-dns chart does not enable a
CRD source for this zone, so the nested wildcard rides on the existing
`*.home-ops` wildcard A automation (§9.4, Gateway LB target). Verify host
resolution (`dig cache.dragonfly.home-ops.yansyah.my.id`) before sending
client traffic over TLS.

## Telemetry-off / monitoring / updates

- Telemetry evidence: the pulled chart `values.yaml` contains no phone-home,
  analytics, or usage-reporting knobs (checked at authoring time); the
  operator exposes only a local metrics endpoint (`:8080` behind the
  `kube-rbac-proxy` sidecar on `:8443`).
- Monitoring: no `ServiceMonitor` objects are shipped and the chart's
  `serviceMonitor.enabled`/`grafanaDashboard.enabled` stay `false` until
  `monitoring.coreos.com` CRDs land (same §9 deviation). Flip: set both to
  `true` once the monitoring stack exists (optionally filling
  `serviceMonitor.labels` with the Prometheus selector).
- Operator bumps flow through `update-policies/dragonfly.yaml` + PR
  automation.

## Environments

`prd` and `stg` currently inherit `../base` unchanged (same shape
as cert-manager before per-env divergence). Per-env tuning (replica count,
snapshot schedule, storage size) lands with the first real instance, not here.
