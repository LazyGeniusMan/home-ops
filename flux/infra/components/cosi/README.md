# COSI (Container Object Storage Interface)

Object-storage provisioning on SeaweedFS: central COSI controller v0.2.2
(`objectstorage.k8s.io/v1alpha1`, image
`gcr.io/k8s-staging-sig-storage/objectstorage-controller:v0.2.2`) plus the
SeaweedFS COSI driver (driver + RBAC + classes in the seaweedfs
component's `configs/base/`), with default `BucketClass/seaweedfs` +
`BucketAccessClass/seaweedfs-key`.

`BucketClaim`/`BucketAccess` pairs colocate with their consumers (one pair
per live bucket, 9 claims):

- Owner claims: cnpg `cnpg-backups`, dragonfly `dragonfly-backups`,
  clickhouse `clickhouse`.
- Per-instance claims: zitadel `zitadel-db`/`zitadel-cache`/`zitadel-assets`,
  coder `coder-db`, clickstack `ferretdb`/`clickstack`.

## Layout

`crds/base/` (5 vendored v0.2.2 CRDs, fleet prune:false `infra-crds`) +
`controllers/base/` (`namespace`/`sa`/`rbac`/`deployment` + HPA/VPA,
CRD-free so `infra-controllers` keeps prune:true) + `configs/base/`
(`resources: []` placeholder -- driver lives in the seaweedfs component).
Dev/prd inherit `../base` unchanged.

## Sources (all vendored, no remote kustomize URLs)

- `crds/base/objectstorage.k8s.io_*.yaml` (5 CRDs v1alpha1) +
  `controllers/base/deployment.yaml` / `sa.yaml` / `rbac.yaml` /
  `namespace.yaml`: from
  `kubernetes-sigs/container-object-storage-interface` tag `v0.2.2`. Re-vendor
  all 5 CRD files together to bump (never hand-edit); pins + caps move
  together (`update-policies/cosi.yaml` + `seaweedfs.yaml`, all `<0.3.0`).
- Controller adapted: namespace `system`->`cosi`, leader-election
  Role/Binding + ClusterRoleBinding subjects `default`->`cosi`.
- Driver + classes in the seaweedfs component mirror the upstream seaweedfs
  chart `templates/cosi/` (plain in-cluster gRPC, no auth/TLS branches).
  Driver image `ghcr.io/seaweedfs/seaweedfs-cosi-driver:v0.3.1`; sidecar
  `gcr.io/k8s-staging-sig-storage/objectstorage-sidecar:v0.2.2`.
- `bucketclasses.yaml` mirrors the chart's `cosi-bucket-class.yaml`
  (`BucketClass/seaweedfs` deletionPolicy Delete +
  `BucketAccessClass/seaweedfs-key` Key) plus `parameters: {replication:
  "001", disk: ssd}`.

## Flow

1. Consumer creates `BucketClaim` (`bucketClassName: seaweedfs`) -> central
   controller creates cluster-scoped `Bucket` (`driverName:
   seaweedfs.objectstorage.k8s.io`).
2. Driver sidecar dials the driver over `unix:///var/lib/cosi/cosi.sock`
   -> `DriverCreateBucket` creates the bucket via filer gRPC
   (`SEAWEEDFS_FILER=seaweed-main-filer.seaweedfs:8888`).
3. Consumer creates `BucketAccess` (`bucketAccessClassName: seaweedfs-key`)
   -> `DriverGrantBucketAccess` mints S3 keys into a Secret
   (`secretS3.endpoint=http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`,
   `region=us-east-1`).
4. Pod mounts the Secret (`secretName`) as a volume.

## Endpoint contract

In-cluster traffic uses `http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`
(plain HTTP port 8333). Dragonfly's `--s3_endpoint` takes the bare host
(`seaweed-main-s3.seaweedfs:8333`) plus `--s3_use_https=false`. The public
`https://s3.seaweedfs.<domain>` Gateway route is for outside-cluster users
only.

## Bucket naming

The driver provisions the live bucket under a controller-generated name
(`bc-<uuid>`), so claim/access names do not equal backing SeaweedFS bucket
names -- read the live name from the claim's `status.bucketName` once
`status.bucketReady` is true.

## Credential bridge

Each `BucketAccess` mints keys into its `credentialsSecretName` Secret in
the claim namespace as BucketInfo JSON
(`secretS3.endpoint/region/accessKeyID/accessSecretKey`). Consumers read
keys through in-namespace Kubernetes-provider stores (`cosi-keys.yaml`
beside each `bucketclaims.yaml`) with GJSON
(`BucketInfo.spec.secretS3.accessKeyID/accessSecretKey`):

| Claim ns | Claim | Creds Secret | Store | Consumer ExternalSecret |
|---|---|---|---|---|
| `cnpg` | `cnpg-backups` | `cnpg-backups-cosi-creds` | `cnpg-cosi` | `cnpg-s3-credentials` |
| `dragonfly` | `dragonfly-backups` | `dragonfly-backups-cosi-creds` | `dragonfly-cosi` | `dragonfly-s3-credentials` |
| `clickhouse` | `clickhouse` | `clickhouse-cosi-creds` | `clickhouse-cosi` | `clickhouse-s3-backup` |
| `zitadel` | `zitadel-db` | `zitadel-db-cosi-creds` | `zitadel-cosi` | `cnpg-s3-credentials` |
| `zitadel` | `zitadel-cache` | `zitadel-cache-cosi-creds` | `zitadel-cosi` | `dragonfly-s3-credentials` |
| `zitadel` | `zitadel-assets` | `zitadel-assets-cosi-creds` | `zitadel-cosi` | `zitadel-asset-storage` |
| `coder` | `coder-db` | `coder-db-cosi-creds` | `coder-cosi` | `cnpg-s3-credentials` |
| `clickstack` | `ferretdb` | `ferretdb-cosi-creds` | `clickstack-cosi` | `cnpg-s3-credentials` |
| `clickstack` | `clickstack` | `clickstack-cosi-creds` | `clickstack-cosi` | `clickhouse-s3-backup` |

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | 1 (central controller + driver singletons) |
| `prd` | 2 (controller + driver HA once multi-node) |

## Telemetry / monitoring / updates

No phone-home knobs in the vendored manifests (controller takes only `--v`;
driver takes only endpoint env). Images auto-track via
`update-policies/cosi.yaml` (controller, `>=0.2.2 <0.3.0`) +
`seaweedfs.yaml` (sidecar + driver), capped `<0.3.0`; bumps hand-bump pins
+ ranges together.
