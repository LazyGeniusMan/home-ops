# SeaweedFS

Operator-managed distributed storage: Enterprise v4.47 image + CSI driver
v1.4.32 (pods mount filer paths via plain PVCs), with tiered StorageClasses
and S3 + filer-UI Gateway routes.

## Layout

`controllers/base/seaweedfs.yaml` (chartproxy OCIRepository + HelmRelease pair:
operator chart 0.1.40, CSI chart 0.2.36) + `controllers/{base,dev,prd}` and
`configs/{base,dev,prd}` overlays; tenant `infra/seaweedfs` via
`flux/infra/update-policies/seaweedfs.yaml`.

## Chart source (chartproxy OCI, classic-only upstream)

Upstream publishes only a classic `index.yaml`, so charts resolve via chartproxy
(`oci://chartproxy.container-registry.com/seaweedfs.github.io/seaweedfs-operator/`
+ `.../seaweedfs-csi-driver/helm`, proxying
`https://seaweedfs.github.io/seaweedfs-operator/`). Chart `version:` pins
hand-bump with ImagePolicy ranges aligned; container images auto-track via
`$imagepolicy` markers. The three hand-vendored files (`driver.yaml`,
`driver-rbac.yaml`, `bucketclasses.yaml` in `configs/base/`) mirror upstream
`templates/cosi/` (the main chart's `cosi.enabled` stanza assumes a
chart-managed cluster).

## Topology

`configs/base/cluster.yaml` (`Seaweed/seaweed-main`): `volume` (nvme tier) 3
replicas on `local-ssd-nvme`; `volumeTopology.sata-bulk` 3 replicas on
`local-ssd-sata`; `master` 3 + `filer` 2 (embedded S3 plus standalone `s3`
gateway Deployment, 2 replicas, port 8333). Services:
`seaweed-main-filer:8888` (filer UI/HTTP), `seaweed-main-s3:8333` (S3 API +
embedded IAM).

## S3 contract

- In-cluster: `http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333`
  (ClusterIP S3 API, plain HTTP) — every in-cluster consumer uses this.
  Dragonfly takes the bare host plus `--s3_use_https=false`.
- External: `https://s3.seaweedfs.home-ops.yansyah.my.id` (443 via the shared
  Gateway) for outside-cluster users only.

Path-style buckets only; buckets are COSI-managed (`BucketClaim` /
`BucketAccess` colocated with each consumer — one pair per bucket:
`cnpg-backups`, `dragonfly-backups`, `clickhouse`,
`zitadel-db`/`zitadel-cache`/`zitadel-assets`, `coder-db`,
`ferretdb`/`clickstack`). Credentials: S3 identities stored in Proton Pass
under `pass://<cluster>/seaweedfs/<consumer>/...` and consumed through an
`ExternalSecret` in each consumer namespace.

## Auth (Zitadel OIDC, admin-only UI)

- `ui-auth.yaml`: namespace-local oauth2-proxy (chart 10.7.0, app v7.15.4,
  Service `ui-auth:4180`) in reverse-proxy mode in front of the filer. Issuer
  `https://admin.zitadel.home-ops.yansyah.my.id`, own `seaweedfs` client, scopes
  `openid profile email groups`, admin-only via
  `--allowed-group=seaweedfs-admin` (project-scoped role, managed by the
  `seaweedfs-sso` Terraform CR). Credentials flow from stored outputs (module
  outputs into `seaweedfs-sso-outputs` via `writeOutputsToSecret`, consumed
  through the in-cluster `seaweedfs-k8s` SecretStore); the cookie secret is
  generated in-Tofu. Markers `infra:seaweedfs-ui-auth-chart` +
  `infra:seaweedfs-ui-auth` (own policy, not the shared apps markers).
- `terraform.yaml` + `terraform/`: the `seaweedfs-sso` CR owns this component's
  Zitadel slice (project + `seaweedfs-admin`/`seaweedfs-user` roles + admin
  grant + OIDC client). Org/admin IDs mirror from the FirstInstance handoff via
  the ESO-synced `seaweedfs-terraform-vars` Secret — no org_id literal in git
  (mirror runs through the narrow Role/RoleBinding in
  `configs/base/zitadel-handoff-rbac.yaml`).
- `s3-ui-routes.yaml`: filer UI reachable only through `ui-auth`; the S3 API
  route stays direct (browser-cookie OIDC would break SigV4 clients).
- In-cluster clients use the direct Services and never hairpin through the
  public hostnames.

## Gateway + DNS

`s3-ui-routes.yaml`: `admin.seaweedfs....` (filer UI via `ui-auth`) +
`s3.seaweedfs....` (S3 API direct), each HTTP->HTTPS 301 + TLS route on the
shared `Gateway/main`. `wildcard-certificate.yaml` mints the
`*.seaweedfs.home-ops.yansyah.my.id` Certificate via
`ClusterIssuer/letsencrypt`. DNS: external-dns watches `gateway-httproute`
sources, so hostnames sync automatically once the Gateway has its LB address.

## StorageClasses

`seaweedfs-ssd-nvme` / `seaweedfs-ssd-sata` (provisioner
`seaweedfs-csi-driver`, collections `ssd-nvme` / `ssd-sata`, replication `000`).
Neither is default — consumers opt in explicitly. The CSI HelmRelease points at
`seaweed-main-filer.seaweedfs:8888`.

## Credentials

`ExternalSecret/cloudflare-api-token` (DNS-01 for the wildcard) syncs from the
same Proton Pass entry as cert-manager. No S3 keys live in this component.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | all 1 (master 1, volumes 1, filer 1, s3 1, ui-auth HPA 1-2, driver HPA 1-2) | hostnames, OIDC wiring, SSO vars, token vault, wildcard DNS + counts -> 1 |
| `prd` | master 3, volumes 3, filer 2, s3 2, ui-auth HPA 2-4, driver HPA 2-4 | hostnames, OIDC wiring, SSO vars, token vault, wildcard DNS + production counts |

## Telemetry / monitoring / updates

Operator `serviceMonitor` + `grafanaDashboard` off; no usage-reporting knobs in
either chart. Health via kubelet + kube-state-metrics. Operator chart 0.1.40 +
CSI chart 0.2.36 hand-bumped; images auto-track via
`update-policies/seaweedfs.yaml`. Sidecar/driver caps move together with
`cosi.yaml`.
Changelog: https://github.com/seaweedfs/seaweedfs/releases.
