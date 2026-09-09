# COSI (Container Object Storage Interface)

GitOps-managed object-storage provisioning on SeaweedFS: the central COSI
controller (release-0.2, `objectstorage.k8s.io/v1alpha1`) plus the SeaweedFS
COSI driver, with default `BucketClass/seaweedfs` +
`BucketAccessClass/seaweedfs-key`. `BucketClaim`/`BucketAccess` pairs are
colocated with their consumers (one pair per live bucket: `cnpg-backups`,
`dragonfly-backups`, `clickhouse`, `rclone-vault`):

- `flux/infra/components/cnpg/configs/base/bucketclaims.yaml`
- `flux/infra/components/dragonfly/configs/base/bucketclaims.yaml`
- `flux/infra/components/clickhouse/configs/base/bucketclaims.yaml`
- `flux/apps/components/rclone/base/bucketclaims.yaml`

Driver, RBAC, and classes stay central in the seaweedfs component's
`configs/base/` (`driver.yaml`, `driver-rbac.yaml`, `bucketclasses.yaml`)
— the driver is a SeaweedFS workload, so it is hosted in the seaweedfs
tenant namespace (moved there from this component).

## Layout (mirrors cert-manager §9 pattern)

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
`storage.xml`, rclone `RCLONE_CONFIG_SW_ENDPOINT`) take the full
`http://…:8333` URL while Dragonfly's `--s3_endpoint` takes the bare host
(`seaweed-main-s3.seaweedfs:8333`) plus `--s3_use_https=false` (its
`s3_use_https` flag defaults true). The public
`https://s3.seaweedfs.<domain>` Gateway route stays for outside-cluster
user access only — no in-cluster consumer may point at it (hairpin +
needless TLS termination).

## Bucket cutover (COSI-provisioned names differ)

The driver provisions the live bucket under a controller-generated name
(`bc-<uuid>`, from `DriverCreateBucket = req.GetName()`), so the claims do
NOT adopt the pre-existing out-of-band buckets (`cnpg-backups`,
`dragonfly-backups`, `clickhouse`, `rclone-vault` — one day to be deleted).
Cutover per bucket: read the live name from the claim's
`status.bucketName`, copy data (`rclone sync` against the internal
endpoint, or SeaweedFS `s3.copy`), repoint the consumer at the new
bucket/prefix, then delete the legacy bucket.

## Credential bridge (COSI secret → consumers)

Each `BucketAccess` mints keys into its `credentialsSecretName` Secret in
the CLAIM namespace (the namespace the claim lands in via the Fleet
`targetNamespace` — `cnpg`, `dragonfly`, `clickhouse`, `rclone` — NOT a
`cosi` namespace) as a `BucketInfo` JSON file (`secretS3.endpoint/region/
accessKeyID/accessSecretKey`). Consumers read those keys through ESO
Kubernetes-provider stores with GJSON `property`
(`BucketInfo.spec.secretS3.accessKeyID/accessSecretKey`):

- Same-namespace (in-namespace `SecretStore` + `eso-k8s-reader` SA/Role
  colocated with the claim): `cnpg-s3-credentials` (cnpg),
  `dragonfly-s3-credentials` (dragonfly), `clickhouse-s3-backup`
  (clickhouse), `rclone-cosi-s3` (rclone).
- Cross-namespace sharers — same bucket, own prefix, NO per-namespace
  claim (a second claim would fork a second `bc-<uuid>` bucket):
  `ClusterSecretStore/cosi-cnpg` serves `cnpg-s3-credentials` in
  `zitadel` (zitadel-db), `coder` (coder-db), and `clickstack`
  (ferretdb); `cosi-dragonfly` serves `dragonfly-s3-credentials` in
  `zitadel` (zitadel-cache); `cosi-clickhouse` serves
  `clickhouse-s3-backup` in `clickstack` (CHI). The stores pin the
  claim-namespace `eso-k8s-reader` SA (least-privilege Role already covers
  the `*-cosi-creds` Secret + `selfsubjectrulesreviews` create, so no new
  RBAC) with `caProvider.namespace` set (mandatory on a ClusterSecretStore).

Target literal keys are unchanged everywhere, so no consumer workload
manifest changed — only the ExternalSecret `secretStoreRef`/`remoteRef`.
The Proton Pass `s3-*`/`sw-*` entries stay seeded in the vault as rollback
(each migrated file documents the repoint).

## Environments

`dev`/`stg`/`prd` inherit `../base` unchanged (same shape as cert-manager
before per-env divergence). Per-env class tuning (e.g. replication per site)
lands here when the second site exists.

## Telemetry-off / monitoring / updates

- No phone-home/usage-reporting knobs in the vendored manifests (controller
  takes only `--v`; driver takes only endpoint env); metrics endpoints, if
  any, are unscraped until the monitoring stack lands (same §9 deviation).
- Images auto-track via `update-policies/cosi.yaml` + PR automation, except
  the controller/sidecar staging pins (date-stamped, non-semver —
  hand-bumped until a `v0.2.x` release tag lands; then switch pins + policy
  ranges to it).
