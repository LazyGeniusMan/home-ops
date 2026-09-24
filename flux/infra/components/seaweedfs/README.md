# SeaweedFS (§12.1)

Operator-managed distributed storage: Enterprise v4.47 image + CSI driver
v1.4.32 (pods mount filer paths via plain PVCs), with tiered
StorageClasses and S3 + filer-UI Gateway routes.

## Layout (mirrors cert-manager §9 pattern)

`controllers/base/seaweedfs.yaml` (chartproxy OCIRepository + HelmRelease
pair for the operator chart 0.1.40 and the CSI chart 0.2.36) +
`controllers/{base,dev,prd}` and `configs/{base,dev,prd}`
overlays; tenant is `infra/seaweedfs` via
`flux/infra/update-policies/seaweedfs.yaml`.

## COSI driver sourcing

The main `seaweedfs` chart's `cosi.enabled` stanza assumes a chart-managed
cluster; this component runs the operator model, so the three hand-vendored
files (`driver.yaml`, `driver-rbac.yaml`, `bucketclasses.yaml` in
`configs/base/`) mirror upstream `templates/cosi/` instead.

## Chart source (chartproxy OCI — classic-only upstream)

Upstream publishes only a classic `index.yaml` (no OCI artifacts), so the
charts are consumed as OCI via chartproxy
(`oci://chartproxy.container-registry.com/seaweedfs.github.io/seaweedfs-operator/`
+ `.../seaweedfs-csi-driver/helm`, proxying the official classic repo
`https://seaweedfs.github.io/seaweedfs-operator/`). Chart `version:` pins
are hand-bumped with the `ImagePolicy` ranges kept aligned; container
images (Enterprise, CSI plugin/mount) still auto-track via `$imagepolicy`
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

Two endpoints, split by caller location:

- In-cluster: `http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`
  (ClusterIP S3 API, plain HTTP, FQDN) — every in-cluster consumer MUST
  use this (no hairpin, no Gateway TLS). Dragonfly takes the bare host
  (`--s3_endpoint` has no scheme) plus `--s3_use_https=false`.
- External: `https://s3.seaweedfs.home-ops.yansyah.my.id` — S3 API on 443
  (terminated by the shared Gateway) for outside-cluster user access only.

Path-style buckets only; every bucket consumer needs an IAM identity:

- Buckets are COSI-managed (`BucketClaim`/`BucketAccess` colocated with
  each consumer — one pair per bucket: `cnpg-backups` (cnpg),
  `dragonfly-backups` (dragonfly), `clickhouse` (clickhouse) plus
  dedicated per-instance claims `zitadel-db`/`zitadel-cache`/`zitadel-assets`
  (zitadel), `coder-db` (coder), `ferretdb`/`clickstack` (clickstack).
  No consumer shares an owner's claim — each writes to its own bucket
  via its own claim (see the cosi README for the claim-per-bucket rule).
- Credentials: create the S3 identity via the operator's S3 config and
  store it in Proton Pass under
  `pass://acme-prd-bdo1-talos-apps-01/seaweedfs/<consumer>/…`
  (`s3-access-key`, `s3-secret-key`); clusters consume them through an
  `ExternalSecret` in their own namespace (same pattern as the other
  components). Never commit keys.

## Auth (Zitadel OIDC, admin-only UI)

- `ui-auth.yaml`: namespace-local oauth2-proxy (official OCI chart
  `oci://ghcr.io/oauth2-proxy/charts/oauth2-proxy:10.7.0`, app `v7.15.4`,
  release `ui-auth` + ClusterIP Service `ui-auth:4180`) in reverse-proxy
  mode in front of the filer. Locked contract per
  `flux/infra/components/zitadel/README.md`: issuer
  `https://admin.zitadel.home-ops.yansyah.my.id`, own `seaweedfs` client
  (redirect `https://<ui_host>/oauth2/callback`, per-app `ui_host`
  wiring in this component's `terraform/`), scopes `openid
  profile email groups`, cookie domain `.home-ops.yansyah.my.id`.
  Admin-only via `--allowed-group=seaweedfs-admin` against the `groups`
  claim (project-scoped role, managed by the `seaweedfs-sso` Terraform
  CR — never implies org admin). Credentials (client id/secret +
  cookie secret) flow end-to-end from stored outputs: the `seaweedfs-sso`
  module outputs all three into the `seaweedfs-sso-outputs` Secret (CR
  `writeOutputsToSecret`), and the `ui-auth-credentials` ExternalSecret
  consumes them from that Secret through the in-cluster `seaweedfs-k8s`
  SecretStore (ESO Kubernetes provider, `eso-k8s-reader` SA + Role) —
  no Proton Pass seeding, never Git. The cookie secret is generated
  in-Tofu (`random_bytes`, 32 bytes base64). Chart + image auto-track
  `flux/infra/update-policies/seaweedfs.yaml` (markers
  `infra:seaweedfs-ui-auth-chart` + `infra:seaweedfs-ui-auth` — per-copy own
  policy, not the shared apps: markers).
- `terraform.yaml` + `terraform/`: the `seaweedfs-sso` CR owns this
  component's Zitadel slice (project + project-scoped roles
  `seaweedfs-admin`/`seaweedfs-user` + the admin grant + the `seaweedfs` OIDC
  client; admin-only UI, no user grant). Upstream IDs (org_id + admin user ID)
  mirror from the FirstInstance handoff (`zitadel-bootstrap-outputs` Secret,
  zitadel ns, operator-created once per the zitadel README runbook) via the
  ESO-synced `seaweedfs-terraform-vars` Secret (same-namespace `varsFrom` +
  the cross-namespace `seaweedfs-zitadel` SecretStore) — no org_id literal in
  git, no email data-source lookups, no remote-state read. The mirror runs
  through the narrow Role/RoleBinding in
  `configs/base/zitadel-handoff-rbac.yaml` (explicit
  `metadata.namespace: zitadel`, get/list/watch on the two handoff Secrets
  only). Provider auth (`jwt_profile_json`) mirrors from the chart-kept
  `zitadel-bootstrap-credentials` the same way — zero-UI, no Proton Pass
  for any SSO input.
- `s3-ui-routes.yaml`: `seaweedfs-ui-tls` points at the `ui-auth`
  Service — the filer UI is reachable ONLY through the proxy. The S3
  API route (`seaweedfs-s3-tls`) stays DIRECT to `seaweed-main-s3:8333`
  on purpose: S3 is a SigV4-gated machine endpoint (access/secret keys
  distributed via ESO from Proton Pass), and browser-cookie OIDC would
  break SigV4 clients (the CNPG/clickhouse/dragonfly backup
  writers address the public S3 hostname directly). Admin-only on S3 is
  enforced by credential distribution, not by the Gateway.
- In-cluster clients SHOULD use the direct Services
  (`seaweed-main-filer:8888`, `seaweed-main-s3:8333`) and never hairpin
  through the public hostnames.

## Gateway + DNS (§8 coordination)

- `s3-ui-routes.yaml`: `admin.seaweedfs.…` (filer UI via `ui-auth`) +
  `s3.seaweedfs.…` (S3 API direct), each HTTP→HTTPS 301 + TLS route on
  the shared `Gateway/main`, `RequestRedirect` filters included so port
  80 stays a redirect source.
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

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | all 1 (master 1, volume nvme 1, volume sata-bulk 1, filer 1, s3 1, ui-auth 1, driver 1 — single-instance, no quorum) | UI/S3 hostnames, proxy OIDC wiring, SSO vars, token vault, wildcard DNS + every `replicas` → 1 |
| `prd` | master 3, volume nvme 3, volume sata-bulk 3, filer 2, s3 2, ui-auth 2, driver 1 (recommended production) | UI/S3 hostnames, proxy OIDC wiring, SSO vars, token vault, wildcard DNS + production counts pinned |

Base holds the full cluster shape; controllers track `../base` with no
patches. The COSI driver stays a singleton (1) in every env — never
scale it.

Upstream reference (read-only): `/tmp/home-ops-docs/seaweedfs-docs` (+
`seaweedfs-operator-docs`, `seaweedfs-cosi-docs`, `seaweedfs-csi-docs`
— CSI `HelmRelease` pins mirror that chart).

## Telemetry-off / monitoring / updates

- Operator `serviceMonitor` + `grafanaDashboard` disabled (unguarded
  monitors OFF); no usage-reporting knobs exist in either chart.
  Evidence: operator `values.yaml` exposes only `serviceMonitor.enabled`
  / `grafanaDashboard.enabled` (both false here); CSI chart 0.2.36 has no
  metrics/telemetry values at all.
- Health is observed via kubelet + kube-state-metrics.
- Operator chart 0.1.40 + CSI chart 0.2.36 are hand-bumped (see Chart
  source above); images auto-track via `update-policies/seaweedfs.yaml`.

## Upgrade runbook

- Version source: chart `version:` pins in
  `controllers/base/seaweedfs.yaml` (operator 0.1.40 + CSI 0.2.36,
  classic repo — hand-bumped, see Chart source above) plus the
  Enterprise / CSI / COSI sidecar+driver image markers in
  `configs/base/cluster.yaml` + `configs/base/driver.yaml`.
- Changelog (server/driver/CSI):
  https://github.com/seaweedfs/seaweedfs/releases.
- Bump: set both chart `version:` pins by hand, let the image
  ImagePolicy PRs land (`update-policies/seaweedfs.yaml`), and move the
  sidecar/driver caps (`<0.3.0`) together with `cosi.yaml` (see the cosi
  README — never PR past the v0.2 line alone).
- Verify: filer UI loads, the S3 endpoint answers (`aws s3 ls` against
  the internal endpoint), and a test PVC provisions + mounts.
