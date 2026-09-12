# COSI (Container Object Storage Interface)

GitOps-managed object-storage provisioning on SeaweedFS: the central COSI
controller (release-0.2, `objectstorage.k8s.io/v1alpha1`) plus the SeaweedFS
COSI driver, with default `BucketClass/seaweedfs` +
`BucketAccessClass/seaweedfs-key`. `BucketClaim`/`BucketAccess` pairs are
colocated with their consumers (one pair per live bucket — 8 claims: 3
owner claims + 5 dedicated per-instance claims):

- Owner claims: `flux/infra/components/cnpg/configs/base/bucketclaims.yaml`
  (`cnpg-backups`), `flux/infra/components/dragonfly/configs/base/bucketclaims.yaml`
  (`dragonfly-backups`), `flux/infra/components/clickhouse/configs/base/bucketclaims.yaml`
  (`clickhouse`).
- Dedicated per-instance claims: `flux/infra/components/zitadel/configs/base/bucketclaims.yaml`
  (`zitadel-db`, `zitadel-cache`),
  `flux/apps/components/coder/base/bucketclaims.yaml` (`coder-db`),
  `flux/apps/components/clickstack/base/bucketclaims.yaml` (`ferretdb`,
  `clickstack`).

Driver, RBAC, and classes stay central in the seaweedfs component's
`configs/base/` (`driver.yaml`, `driver-rbac.yaml`, `bucketclasses.yaml`)
— the driver is a SeaweedFS workload, so it is hosted in the seaweedfs
tenant namespace.

## Layout (mirrors the cert-manager component pattern)

`controllers/base/` (vendored release-0.2 CRDs + central controller) +
`configs/base/` (driver RBAC + Deployment + classes) + plain `../base`
passthroughs in `controllers/{dev,stg,prd}` and `configs/{dev,stg,prd}`;
tenant is `infra/cosi` via `flux/infra/update-policies/cosi.yaml`.

## Sources (all vendored — no remote kustomize URLs)

- `controllers/base/objectstorage.k8s.io_*.yaml` (5 CRDs: BucketClass,
  BucketClaim, Bucket, BucketAccessClass, BucketAccess, v1alpha1,
  controller-gen v0.17.3) + `deployment.yaml` / `sa.yaml` / `rbac.yaml` /
  `namespace.yaml`: from `kubernetes-sigs/container-object-storage-interface`
  @ `origin/release-0.2` (`f75d4750`). Do NOT track `main` (v1alpha2,
  incompatible with this driver).
- Controller image
  `gcr.io/k8s-staging-sig-storage/objectstorage-controller:v20250905-controllerv0.2.0-rc1-100-gd904c62`:
  staging build of the release-0.2 branch. No `registry.k8s.io` promotion
  exists yet (`RELEASE.md` keeps template-project boilerplate; `cloudbuild`
  publishes staging only). Adapted from upstream: namespace `system`→`cosi`
  (matches the Flux tenant namespace), leader-election Role/Binding +
  ClusterRoleBinding subjects `default`→`cosi`.
- `configs/base/driver.yaml` + `driver-rbac.yaml`: mirror the upstream
  `seaweedfs` chart `templates/cosi/` (`cosi-deployment.yaml`,
  `cosi-cluster-role.yaml`, `cosi-service-account.yaml`), minus the
  auth/TLS branches (plain in-cluster gRPC). Driver image
  `ghcr.io/seaweedfs/seaweedfs-cosi-driver:v0.1.2` (matches the chart
  default; `quay.io/seaweedfs` repo is disabled/unauthenticated). Sidecar
  `gcr.io/k8s-staging-sig-storage/objectstorage-sidecar:v20250711-controllerv0.2.0-rc1-80-gc2f6e65`
  (the chart's pin — the release-0.2 counterpart of the driver's
  `provisioner-sidecar v0.1.0` library).
- `configs/base/bucketclasses.yaml`: mirrors the chart's
  `cosi-bucket-class.yaml` (`BucketClass/seaweedfs` deletionPolicy Delete +
  `BucketAccessClass/seaweedfs-key` authenticationType Key), plus
  `parameters: {replication: "001", disk: ssd}` (driver keys per
  `pkg/driver/provisioner.go`: 3-digit DC/rack/node placement + FilerConf
  disk-type tag).

## COSI flow

1. Consumer creates `BucketClaim` (namespaced, `bucketClassName: seaweedfs`)
   → central controller creates the cluster-scoped `Bucket`
   (`driverName: seaweedfs.objectstorage.k8s.io`, parameters copied from the
   class).
2. Driver sidecar's Bucket listener dials the driver over
   `unix:///var/lib/cosi/cosi.sock` (shared `emptyDir`) →
   `DriverCreateBucket` creates the bucket in SeaweedFS via the filer gRPC
   (`SEAWEEDFS_FILER=seaweed-main-filer.seaweedfs:8888`, same endpoint as the
   CSI HelmRelease; FilerConf written only when `disk`/`replication` set).
3. Consumer creates `BucketAccess` (`bucketAccessClassName: seaweedfs-key`)
   → `DriverGrantBucketAccess` mints S3 keys into a Secret
   (`secretS3.endpoint=http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`
   — internal ClusterIP, no hairpin; `region=us-east-1`, ignored by
   SeaweedFS but required by some S3 clients).
4. Pod mounts the Secret (`secretName`) as a volume (see the driver's
   `examples/consumer-pod.yaml`).

Prereqs (verified, not managed here): `seaweedfs` operator chart 0.1.40 +
`Seaweed/seaweed-main` v4.45 with filer (`seaweed-main-filer.seaweedfs:8888`)
and S3 (`seaweed-main-s3.seaweedfs.svc.cluster.local:8333`).

## Endpoint contract (internal vs external)

In-cluster traffic MUST use the internal S3 service
`http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333` (FQDN —
resolves cluster-wide without relying on CoreDNS search expansion).
Port 8333 + plain HTTP: the S3 Deployment
serves HTTP only, so URL clients (CNPG `endpointURL`, ClickHouse
`storage.xml`, S3 client endpoint env vars) take the full
`http://…:8333` URL while Dragonfly's `--s3_endpoint` takes the bare host
(`seaweed-main-s3.seaweedfs:8333`) plus `--s3_use_https=false` (its
`s3_use_https` flag defaults true). The public
`https://s3.seaweedfs.<domain>` Gateway route stays for outside-cluster
user access only — no in-cluster consumer may point at it (hairpin +
needless TLS termination).

## Bucket naming (COSI-provisioned names differ)

The driver provisions the live bucket under a controller-generated name
(`bc-<uuid>`, from `DriverCreateBucket = req.GetName()`), so claim/access
names do NOT equal the backing SeaweedFS bucket names — read the live name
from the claim's `status.bucketName` once `status.bucketReady` is true
(`aws s3 sync` against the internal endpoint, or SeaweedFS `s3.copy`,
moves data between buckets when re-homing a consumer). The three owner
claims back the long-lived backup buckets (`cnpg-backups`,
`dragonfly-backups`, `clickhouse`); each dedicated
per-instance claim backs its own bucket (e.g. `zitadel-db` backs Zitadel's
Postgres WAL + base backups).

## Credential bridge (COSI secret → consumers)

Each `BucketAccess` mints keys into its `credentialsSecretName` Secret in
the CLAIM namespace (the namespace the claim lands in via the Fleet
`targetNamespace` — `cnpg`, `dragonfly`, `clickhouse`,
`zitadel`, `coder`, `clickstack` — NOT a
`cosi` namespace) as a `BucketInfo` JSON file (`secretS3.endpoint/region/
accessKeyID/accessSecretKey`). Consumers read those keys through ESO
Kubernetes-provider stores with GJSON `property`
(`BucketInfo.spec.secretS3.accessKeyID/accessSecretKey`):

- Every claim has an in-namespace `SecretStore` + `eso-k8s-reader`
  SA/Role colocated with it (the `cosi-keys.yaml` beside each
  `bucketclaims.yaml`). Consumers only ever read keys from their own
  namespace's store:
  | Claim ns | Claim | Creds Secret | Store | Consumer ExternalSecret |
  |---|---|---|---|---|
  | `cnpg` | `cnpg-backups` | `cnpg-backups-cosi-creds` | `cnpg-cosi` | `cnpg-s3-credentials` (postgres-base) |
  | `dragonfly` | `dragonfly-backups` | `dragonfly-backups-cosi-creds` | `dragonfly-cosi` | `dragonfly-s3-credentials` (dragonfly-base) |
  | `clickhouse` | `clickhouse` | `clickhouse-cosi-creds` | `clickhouse-cosi` | `clickhouse-s3-backup` (CHI) |
  | `zitadel` | `zitadel-db` | `zitadel-db-cosi-creds` | `zitadel-cosi` | `cnpg-s3-credentials` (zitadel-db) |
  | `zitadel` | `zitadel-cache` | `zitadel-cache-cosi-creds` | `zitadel-cosi` | `dragonfly-s3-credentials` (zitadel-cache) |
  | `coder` | `coder-db` | `coder-db-cosi-creds` | `coder-cosi` | `cnpg-s3-credentials` (coder-db) |
  | `clickstack` | `ferretdb` | `ferretdb-cosi-creds` | `clickstack-cosi` | `cnpg-s3-credentials` (ferretdb) |
  | `clickstack` | `clickstack` | `clickstack-cosi-creds` | `clickstack-cosi` | `clickhouse-s3-backup` (CHI) |

Target literal keys are unchanged everywhere, so no consumer workload
manifest changed — only the ExternalSecret `secretStoreRef`/`remoteRef`.
The Proton Pass `s3-*`/`sw-*` entries stay seeded in the vault as rollback
(each migrated file documents the repoint).

## Environments

`dev`/`stg`/`prd` inherit `../base` unchanged (same shape as cert-manager).
Per-env class tuning (e.g. replication per site)
lands here when the second site exists.

## Telemetry-off / monitoring / updates

- No phone-home/usage-reporting knobs in the vendored manifests (controller
  takes only `--v`; driver takes only endpoint env); metrics endpoints, if
  any, are unscraped until the monitoring stack lands (same as the
  cert-manager component).
- Images auto-track via `update-policies/cosi.yaml` + PR automation, except
  the controller/sidecar staging pins (date-stamped, non-semver —
  hand-bumped until a `v0.2.x` release tag lands; then switch pins + policy
  ranges to it).
