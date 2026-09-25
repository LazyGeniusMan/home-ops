# SeaweedFS

Operator-managed distributed storage: Enterprise v4.47 image + CSI driver
v1.4.32 (pods mount filer paths via plain PVCs), with tiered
StorageClasses and S3 + filer-UI Gateway routes.

## Layout

`controllers/base/seaweedfs.yaml` (chartproxy OCIRepository + HelmRelease
pair: operator chart 0.1.40, CSI chart 0.2.36) + `controllers/{base,dev,prd}`
and `configs/{base,dev,prd}` overlays; tenant `infra/seaweedfs` via
`flux/infra/update-policies/seaweedfs.yaml`.

## Chart source (chartproxy OCI, classic-only upstream)

Upstream publishes only a classic `index.yaml`, so charts resolve via
chartproxy (`oci://chartproxy.container-registry.com/seaweedfs.github.io/seaweedfs-operator/`
+ `.../seaweedfs-csi-driver/helm`, proxying
`https://seaweedfs.github.io/seaweedfs-operator/`). Chart `version:` pins
hand-bump with ImagePolicy ranges aligned; container images (Enterprise,
CSI plugin/mount) auto-track via `$imagepolicy` markers.

The main chart's `cosi.enabled` stanza assumes a chart-managed cluster, so
the three hand-vendored files (`driver.yaml`, `driver-rbac.yaml`,
`bucketclasses.yaml` in `configs/base/`) mirror upstream
`templates/cosi/` instead.

## Topology

`configs/base/cluster.yaml` (`Seaweed/seaweed-main`):

- `volume` (nvme tier): 3 replicas, `local-ssd-nvme`, `rack: nvme`,
  `dataCenter: dc1`.
- `volumeTopology.sata-bulk`: 3 replicas, `local-ssd-sata`, `rack: sata`.
- `master` 3 replicas + `filer` 2 replicas, both on `local-ssd-nvme`;
  filer carries embedded S3 plus a standalone `s3` gateway Deployment (2
  replicas, port 8333).
- Services: `seaweed-main-filer:8888` (filer UI/HTTP),
  `seaweed-main-s3:8333` (S3 API + embedded IAM).

## S3 contract

- In-cluster: `http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`
  (ClusterIP S3 API, plain HTTP) -- every in-cluster consumer uses this.
  Dragonfly takes the bare host plus `--s3_use_https=false`.
- External: `https://s3.seaweedfs.home-ops.yansyah.my.id` (443 via the
  shared Gateway) for outside-cluster users only.

Path-style buckets only; buckets are COSI-managed (`BucketClaim` /
`BucketAccess` colocated with each consumer -- one pair per bucket:
`cnpg-backups`, `dragonfly-backups`, `clickhouse`,
`zitadel-db`/`zitadel-cache`/`zitadel-assets`, `coder-db`,
`ferretdb`/`clickstack`). Credentials: S3 identities stored in Proton Pass
under `pass://<cluster>/seaweedfs/<consumer>/...` and consumed through an
`ExternalSecret` in each consumer namespace.

## Auth (Zitadel OIDC, admin-only UI)

- `ui-auth.yaml`: namespace-local oauth2-proxy (chart 10.7.0, app v7.15.4,
  release `ui-auth`, Service `ui-auth:4180`) in reverse-proxy mode in front
  of the filer. Issuer `https://admin.zitadel.home-ops.yansyah.my.id`, own
  `seaweedfs` client (redirect `https://<ui_host>/oauth2/callback`), scopes
  `openid profile email groups`, cookie domain
  `.home-ops.yansyah.my.id`. Admin-only via `--allowed-group=seaweedfs-admin`
  (project-scoped role, managed by the `seaweedfs-sso` Terraform CR).
  Credentials (client id/secret + cookie secret) flow from stored outputs:
  the module outputs all three into `seaweedfs-sso-outputs`
  (`writeOutputsToSecret`), and `ui-auth-credentials` consumes them through
  the in-cluster `seaweedfs-k8s` SecretStore. The cookie secret is
  generated in-Tofu (32 bytes base64). Markers
  `infra:seaweedfs-ui-auth-chart` + `infra:seaweedfs-ui-auth` (own policy,
  not the shared apps markers).
- `terraform.yaml` + `terraform/`: the `seaweedfs-sso` CR owns this
  component's Zitadel slice (project + `seaweedfs-admin`/`seaweedfs-user`
  roles + admin grant + `seaweedfs` OIDC client). Org/admin IDs mirror from
  the FirstInstance handoff (`zitadel-bootstrap-outputs` Secret) via the
  ESO-synced `seaweedfs-terraform-vars` Secret (same-namespace `varsFrom` +
  cross-namespace `seaweedfs-zitadel` SecretStore) -- no org_id literal in
  git. The mirror runs through the narrow Role/RoleBinding in
  `configs/base/zitadel-handoff-rbac.yaml`. Provider auth
  (`jwt_profile_json`) mirrors from `zitadel-bootstrap-credentials` the
  same way.
- `s3-ui-routes.yaml`: filer UI reachable only through `ui-auth`; the S3
  API route stays direct to `seaweed-main-s3:8333` (SigV4 machine endpoint;
  browser-cookie OIDC would break SigV4 clients).
- In-cluster clients use the direct Services and never hairpin through the
  public hostnames.

## Gateway + DNS

`s3-ui-routes.yaml`: `admin.seaweedfs....` (filer UI via `ui-auth`) +
`s3.seaweedfs....` (S3 API direct), each HTTP->HTTPS 301 + TLS route on the
shared `Gateway/main`. `wildcard-certificate.yaml`:
`Certificate/wildcard-seaweedfs` mints `seaweedfs-wildcard-tls`
(`*.seaweedfs.home-ops.yansyah.my.id`) via `ClusterIssuer/letsencrypt`.
DNS: external-dns watches `gateway-httproute` sources, so hostnames sync
automatically once the Gateway has its LB address.

## StorageClasses

`seaweedfs-ssd-nvme` / `seaweedfs-ssd-sata` (provisioner
`seaweedfs-csi-driver`, collections `ssd-nvme` / `ssd-sata`, replication
`000`). Neither is default -- consumers opt in explicitly. The CSI
HelmRelease points at `seaweed-main-filer.seaweedfs:8888`.

## Credentials

`ExternalSecret/cloudflare-api-token` (DNS-01 for the wildcard) syncs from
the same Proton Pass entry as cert-manager. No S3 keys live in this
component.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | all 1 (master 1, volumes 1, filer 1, s3 1, ui-auth HPA 1-2, driver HPA 1-2) | hostnames, OIDC wiring, SSO vars, token vault, wildcard DNS + every `replicas` -> 1 + HPAs 1 / 2 |
| `prd` | master 3, volumes 3, filer 2, s3 2, ui-auth HPA 2-4, driver HPA 2-4 | hostnames, OIDC wiring, SSO vars, token vault, wildcard DNS + production counts |

## Telemetry / monitoring / updates

Operator `serviceMonitor` + `grafanaDashboard` off; no usage-reporting
knobs in either chart. Health via kubelet + kube-state-metrics. Operator
chart 0.1.40 + CSI chart 0.2.36 hand-bumped; images auto-track via
`update-policies/seaweedfs.yaml` (Enterprise >=4.47, CSI >=1.4.32,
sidecar >=0.2.2 <0.3.0, driver >=0.3.1 <0.4.0, ui-auth image >=7.15.4 +
chart >=10.7.0). Sidecar/driver caps move together with `cosi.yaml`.
Changelog: https://github.com/seaweedfs/seaweedfs/releases.
