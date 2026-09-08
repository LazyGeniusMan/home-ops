# SeaweedFS (§12.1)

Operator-managed distributed storage: Enterprise v4.45 image + CSI driver
v1.4.30 (pods mount filer paths via plain PVCs), with tiered
StorageClasses and S3 + filer-UI Gateway routes.

## Layout (mirrors cert-manager §9 pattern)

`controllers/base/seaweedfs.yaml` (HelmRepository + HelmRelease pair for
the operator chart 0.1.40 and the CSI chart 0.2.36) +
`controllers/{production,staging}` and `configs/{base,production,staging}`
overlays; tenant is `infra/seaweedfs` via
`flux/infra/update-policies/seaweedfs.yaml`.

## Chart source note (OCI unavailable — justified)

`helm pull oci://…` against both seaweedfs Helm hosts returns 403
(verified 2026-09-08): upstream publishes only a classic `index.yaml`
(no OCI artifacts). The component therefore uses `HelmRepository` +
pinned `version:` fields. Consequence: `ImageUpdateAutomation` cannot
bump charts automatically — bump `version:` by hand and keep the
`ImagePolicy` ranges in the update policy aligned. Container images
(Enterprise, CSI plugin/mount) still auto-track via `$imagepolicy`
markers.

## Topology (§9.5 backing classes)

`configs/base/cluster.yaml` (`Seaweed/seaweed-main`):

- `volume` (nvme tier): 3 StatefulSet replicas, `storageClassName:
  local-ssd-nvme`, `rack: nvme`, `dataCenter: dc1` → `/var/mnt/nvme-data`.
- `volumeTopology.sata-bulk`: 3 replicas, `storageClassName:
  local-ssd-sata`, `rack: sata` → `/var/mnt/sata-data`.
- `master` 3 replicas + `filer` 2 replicas, both persisted on
  `local-ssd-nvme`; filer carries embedded S3 (`s3.enabled`) plus a
  standalone `s3` gateway Deployment (2 replicas, port 8333).
- Operator services the routes: `seaweed-main-filer:8888` (filer UI/HTTP),
  `seaweed-main-s3:8333` (S3 API + embedded IAM).

## S3 contract (what other components use)

Endpoint: `https://s3.seaweedfs.home-ops.yansyah.my.id` — S3 API on 443
(terminated by the shared Gateway). Path-style buckets only; every bucket
consumer needs an IAM identity:

- Buckets are created out-of-band (`weed shell` / S3 API against the
  cluster); the convention is one bucket per consumer, e.g.
  `rclone-vault`, `appname-media`.
- Credentials: create the S3 identity via the operator's S3 config and
  store it in Proton Pass under
  `pass://acme-prd-bdo1-talos-apps-01/seaweedfs/<consumer>/…`
  (`s3-access-key`, `s3-secret-key`); clusters consume them through an
  `ExternalSecret` in their own namespace (same pattern as the rclone
  component). Never commit keys.

## Gateway + DNS (§8 coordination)

- `s3-ui-routes.yaml`: `ui.seaweedfs.…` (filer UI) +
  `s3.seaweedfs.…` (S3 API), each HTTP→HTTPS 301 + TLS route on the
  shared `Gateway/main`, `RequestRedirect` filters included so port 80
  stays a redirect source. Route names are stable: wave 3c adds
  Zitadel/OIDC `ExternalAuth` filters in place — **no OIDC wiring in
  this component** (routes ship without it).
- `wildcard-certificate.yaml`: `Certificate/wildcard-seaweedfs` mints
  `seaweedfs-wildcard-tls` (`*.seaweedfs.home-ops.yansyah.my.id`) via
  `ClusterIssuer/letsencrypt`; same in-namespace-credential discipline as
  the gateway-api component.
- DNS: `external-dns` watches `gateway-httproute` sources, so the four
  hostnames above sync to Cloudflare automatically once the Gateway has
  its LB address — no manual records.

## StorageClasses

`seaweedfs-ssd-nvme` / `seaweedfs-ssd-sata` (provisioner
`seaweedfs-csi-driver`, filer collections `ssd-nvme` / `ssd-sata`,
replication `000`). Neither is default — consumers opt in explicitly.
The CSI HelmRelease points at `seaweed-main-filer.seaweedfs:8888` so
provisioned volumes land on this cluster.

## Credentials

`ExternalSecret/cloudflare-api-token` (DNS-01 for the wildcard) syncs
from the same Proton Pass entry as cert-manager. S3 identities: see
"S3 contract" above — no keys live in this component.

## Environments

Base holds the full cluster shape; `production`/`staging` are plain
`../base` passthroughs (capacity/affinity tuning lands here when the
second site exists).

## Telemetry-off / monitoring / updates

- Operator `serviceMonitor` + `grafanaDashboard` disabled (unguarded
  monitors OFF); no usage-reporting knobs exist in either chart.
  Evidence: operator `values.yaml` exposes only `serviceMonitor.enabled`
  / `grafanaDashboard.enabled` (both false here); CSI chart 0.2.36 has no
  metrics/telemetry values at all.
- Health is observed via kubelet + kube-state-metrics until the
  monitoring stack lands; S3 availability can alert on the
  `seaweedfs-s3-tls` route then.
- Operator chart 0.1.40 + CSI chart 0.2.36 are hand-bumped (see chart
  source note); images auto-track via `update-policies/seaweedfs.yaml`.
