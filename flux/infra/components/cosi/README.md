# COSI (Container Object Storage Interface)

Object-storage provisioning on SeaweedFS: central COSI controller v0.2.2 (image `gcr.io/k8s-staging-sig-storage/objectstorage-controller:v0.2.2`) plus the SeaweedFS COSI driver (driver + RBAC + classes in the seaweedfs component's `configs/base/`), with default `BucketClass/seaweedfs` + `BucketAccessClass/seaweedfs-key`.

Consumer `BucketClaim`/`BucketAccess` pairs colocate with their consumers (one pair per live bucket, 9 claims): cnpg `cnpg-backups`, dragonfly `dragonfly-backups`, clickhouse `clickhouse`, zitadel `zitadel-db`/`zitadel-cache`/`zitadel-assets`, coder `coder-db`, clickstack `ferretdb`/`clickstack`.

## Layout

`crds/base/` (first-party `helm-cosi` OCI chart, 5 CRDs v0.2.2, fleet prune:false `infra-crds`) + `controllers/base/` (`namespace`/`sa`/`rbac`/`deployment` + HPA/VPA, CRD-free so `infra-controllers` keeps prune:true) + `configs/base/` (`resources: []` placeholder — driver lives in the seaweedfs component). Dev/prd inherit `../base` unchanged.

## Sources

- `crds/base/cosi-crds.yaml`: `projects/helm-cosi` wraps the 5 upstream CRDs (`objectstorage.k8s.io/v1alpha1`, upstream tag `v0.2.2`, fetched at publish time; no YAML committed).
- `controllers/base/`: from the same tag (namespace adapted `system`->`cosi`, subjects `default`->`cosi`).
- Driver + classes in the seaweedfs component mirror upstream `templates/cosi/` (plain in-cluster gRPC; driver ClusterRole narrowed to read + status-update only). Driver image `ghcr.io/seaweedfs/seaweedfs-cosi-driver:v0.3.1`; sidecar `gcr.io/k8s-staging-sig-storage/objectstorage-sidecar:v0.2.2`.

## Endpoint / naming / credentials

- In-cluster S3: `http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333` (plain HTTP, path-style buckets only). Dragonfly's `--s3_endpoint` takes the bare host plus `--s3_use_https=false`. The public `https://s3.seaweedfs.<domain>` route is for outside-cluster users only.
- The driver provisions the live bucket under a controller-generated name (`bc-<uuid>`) — read it from the claim's `status.bucketName` once `status.bucketReady` is true.
- Each `BucketAccess` mints keys into its `credentialsSecretName` Secret as BucketInfo JSON (`secretS3.endpoint/region/accessKeyID/accessSecretKey`). Consumers read them through in-namespace stores (`cosi-keys.yaml` beside each `bucketclaims.yaml`) with GJSON (`BucketInfo.spec.secretS3.accessKeyID/accessSecretKey`):

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

## Updates

`update-policies/cosi.yaml` + `seaweedfs.yaml` (`>=0.2.2 <0.3.0`, all 5 CRDs bump together; pins + caps move together); CRDs bump via Chart.yaml + wrapper `ref.tag` together (no ImagePolicy/marker, atomic hand-bump), human merges.
