# Zitadel (§11.1)

Zitadel **v4.17.1** identity provider: the OIDC issuer for
`https://zitadel.home-ops.yansyah.my.id` plus the locked client contract the
wave-3c app writers build against (table below — do not deviate).

## Chart source

OCI is the upstream source of truth, verified by pull:

- `oci://ghcr.io/zitadel/zitadel-charts/zitadel`, tag **10.0.4** (digest
  `sha256:9afa657fad65079857339f7d7fd296c73e577f6c8ec4e4a103093be35964b50c`,
  `helm template` + `helm lint` pass locally).
- Chart↔app divergence (cnpg-style, unlike §10.3 dragonfly where versions
  agree): chart **10.0.4** embeds app **v4.15.3**, but the app image is pinned
  separately in values (`image.tag` + `login.image.tag` = **v4.17.1**;
  `ghcr.io/zitadel/zitadel:v4.17.1` and
  `ghcr.io/zitadel/zitadel-login:v4.17.1` manifests verified on GHCR). On
  chart bumps set both tags to the new chart's appVersion together.
- No OCI fallback was needed: the chart repo's own release workflow pushes
  every chart version to exactly this GHCR path (classic repo
  `https://charts.zitadel.com` carries the same charts but is not referenced).
- Local reference: `/tmp/home-ops-docs/zitadel-docs` (upstream
  [zitadel](https://github.com/zitadel/zitadel) branch `main`); d2-infra
  carries no Zitadel component, so the bounded sources were the local docs
  plus the pulled chart `values.yaml`/templates.
- The `update-policies/zitadel.yaml` floor `>=10.0.4` tracks the CHART line
  (ImageRepository + ImagePolicy + `$imagepolicy` marker
  `infra:zitadel:tag`, same §7 contract as cert-manager/cnpg/dragonfly).

## Layout

Mirrors §9/cert-manager file-for-file: `controllers/{base,prd,stg}`
(OCIRepository + HelmRelease, env overlays inherit base unchanged) and
`configs/{base,prd,stg}` (secrets, DB, cache, certificate, routes,
identity intent + Terraform bootstrap CR).

## Dependencies (§§8–10)

- Database: `configs/base/zitadel-db.yaml` — namespace-local instantiation of
  the §10.1 `cluster-base` template (same shape: 3 instances, sync quorum 1,
  `local-ssd-nvme`, continuous WAL + daily base backup to SeaweedFS S3 under
  `s3://cnpg-backups/zitadel/`). Adjusted: dbname/owner `zitadel`, own S3
  prefix. Connection via DSN (`ZITADEL_DATABASE_POSTGRES_DSN` from the
  `zitadel-db-credentials` ExternalSecret, `sslmode=require`).
- Cache: `configs/base/zitadel-cache.yaml` — namespace-local instantiation of
  the §10.3 `dragonfly-base` template (3 replicas, tiered persistence, hourly
  S3 snapshots under `s3://dragonfly-backups/zitadel-cache/`). Connection
  contract per the §10.3 README: host
  `zitadel-cache.zitadel.svc.cluster.local`, port **6379**, no auth.
- Routing: `configs/base/zitadel-httproute.yaml` — two hand-written HTTPRoutes
  on the shared §8.1 Gateway (`main`, cross-namespace parentRef): `/` → the
  `zitadel` Service (8080, h2c) and `/ui/v2/login` → `zitadel-login` (3000).
  Chart-native ingress/gateway templating stays off so the §8.1 pattern is
  owned explicitly. TLS terminates at the Gateway via the in-namespace
  wildcard `Certificate` (`wildcard-certificate.yaml`, same duplicate pattern
  as §8.1 — cert-manager Secrets are namespace-local).
- HTTP/2 note: the console requires end-to-end HTTP/2 — the Gateway must
  forward h2c to the backend (the Service already advertises
  `appProtocol: kubernetes.io/h2c`).

## Credentials

All secrets sync from Proton Pass via ESO (Git holds `remoteRef`s only).
Seed each vault entry with pass-cli:

- `pass://acme-prd-bdo1-talos-apps-01/zitadel/masterkey` — 32-byte masterkey
  (`tr -dc A-Za-z0-9 </dev/urandom | head -c 32`). IMMUTABLE: Zitadel cannot
  re-key; loss means loss of all encrypted data.
- `pass://acme-prd-bdo1-talos-apps-01/zitadel/db-dsn` — full DSN, must embed
  the same password as `db-password` below.
- `pass://acme-prd-bdo1-talos-apps-01/zitadel/db-password` — CNPG app-user
  password (username must equal `initdb.owner`; rotate with the DSN).
- `pass://acme-prd-bdo1-talos-apps-01/zitadel/smtp-*` — relay user/password
  (optional; unwired until a relay exists — Zitadel logs mail links to pod
  output meanwhile).
- S3 keys are COSI-minted, not Proton Pass: `cnpg-s3-credentials`
  (zitadel-db) syncs from the `cnpg-backups-cosi-creds` BucketInfo JSON
  through the `cosi-cnpg` ClusterSecretStore, and
  `dragonfly-s3-credentials` (zitadel-cache) from
  `dragonfly-backups-cosi-creds` through `cosi-dragonfly` (same buckets,
  own prefixes — no per-namespace claims; see the cosi README). The
  `pass://…/{cnpg,dragonfly}/s3-*` vault entries stay seeded as rollback.
- `pass://acme-prd-bdo1-talos-apps-01/cert-manager/cloudflare-api-token` —
  same vault path as §9, copied so the DNS-01 secret exists in this
  namespace too.

## OIDC contract (LOCKED for wave-3c app writers)

App writers build against THIS table — do not deviate.

| Item | Value |
|---|---|
| Issuer | `https://zitadel.home-ops.yansyah.my.id` |
| Org | `home-ops` |
| Users | `admin@home-ops.yansyah.my.id` (super-admin, `admin` group + role), `user@home-ops.yansyah.my.id` (normal, `users` group + role) |
| Groups | `admin` (admin@ member), `users` (user@ member) — asserted in the `groups` claim |
| Scopes (all clients) | `openid profile email groups` |
| Flow (all clients) | Authorization code + PKCE, refresh tokens on |
| Owner: `coder` → client `coder` | `https://coder.home-ops.yansyah.my.id/*` (post-logout → `https://coder.home-ops.yansyah.my.id/`) |
| Owner: `clickstack` → client `clickstack` | `https://clickstack.home-ops.yansyah.my.id/*` (covers the per-instance oauth2-proxy callback under `/oauth2/callback`) |
| Owner: `hubble-ui` → client `hubble` | `https://hubble.home-ops.yansyah.my.id/*` (covers the per-instance oauth2-proxy callback under `/oauth2/callback`) |
| Owner: `flux-operator-ui` → client `flux-operator-ui` | `https://flux-operator.home-ops.yansyah.my.id/*` (covers the per-instance oauth2-proxy callback under `/oauth2/callback`) |
| Owner: `headlamp` → client `headlamp` | `https://headlamp.home-ops.yansyah.my.id/*` |
| Owner: `seaweedfs` → client `seaweedfs` | `https://ui.seaweedfs.home-ops.yansyah.my.id/oauth2/callback` (serves the filer-UI proxy) |

Each app owns its own `zitadel_project` + `zitadel_application_oidc` client
in its per-app `terraform/` slice (own project roles/grants assert the
`groups` claim); the central bootstrap slice owns no clients. Post-logout
redirects point at each app's root (`https://<app>…/`).

## Identity-as-code (Tofu Controller)

Machine-applied by Tofu Controller (`terraforms.infra.contrib.fluxcd.io`
v1alpha2, chart 0.16.5 — see the `tofu-controller` component):

- `configs/base/terraform-bootstrap.yaml` — `zitadel-bootstrap-identity`
  Terraform CR (`approvePlan: auto`, in-cluster state backend) applying the
  bootstrap slice of `terraform/`: `zitadel_org`, `zitadel_human_user` × 2,
  `zitadel_org_member` × 2, legacy `zitadel_project` (`home-ops`) + roles +
  grants. OIDC clients live in each app's own per-app `terraform/` slice —
  this bootstrap slice owns none.
- `terraform/` — the modules (`zitadel/zitadel ~> 3.3`, `tofu validate`
  passes; the provider ships no `zitadel_user_group` resources, so groups map
  to `zitadel_org_member` + `zitadel_project` roles + `zitadel_user_grant`).
  Single source of truth for the contract table above.
- `configs/base/org-users.yaml` — human-readable mirror of the intent (kept
  in sync with `terraform/` + the CR `vars` on contract changes).
- Secrets (`admin_initial_password`, `user_initial_password`,
  `jwt_profile_json`) flow via the ESO `zitadel-terraform-vars` ExternalSecret
  (Proton Pass `pass://<env-vault>/zitadel/terraform-*`, never Git); per-env
  vault paths + `domain`/email `vars` land in the `dev`/`prd`/`stg` overlays.
  Rotate by updating the vault entries — ESO syncs and the next reconcile
  picks them up.
- Outputs: `org_id` + `project_id` + `admin_user_id` + `user_user_id` land
  in the `zitadel-bootstrap-outputs` Secret (same `zitadel` namespace) for
  the later per-app Terraform task. All four are plain IDs (non-sensitive)
  — per-app slices consume them via CR `varsFrom` (Secret → literal `vars`),
  never via ESO/`pass://`. Only JWT/passwords stay in ESO. Consumer pattern:
  read the four IDs out of `zitadel-bootstrap-outputs` into the per-app CR
  `vars` (`org_id`, `admin_user_id`, `user_user_id`, …) instead of looking
  users up by email data source.

One-time prerequisite (manual): the chart has NO FirstInstance bootstrap
stanza, so before the first reconcile provision the IAM_OWNER service user
via the FirstInstance machine user, download its key JSON, and seed the
vault entries above (initial passwords + `terraform-jwt-profile-json`).
Without the key the runner fails auth and retries on interval.

Manual fallback: `terraform init && terraform apply` from `terraform/` with
`-var jwt_profile_json="$(cat <key>.json)"` (+ domain/email `-var`s for dev).
Client secrets live in each app's per-app `terraform/` state — read them into
Proton Pass (never Git).

## Telemetry-off / monitoring / updates

- Telemetry evidence: the pulled chart `values.yaml` contains no phone-home,
  analytics, or usage-reporting knobs (checked at authoring time; a grep for
  telem\*/analytic\*/phone-home/usage-report/tracking returns nothing).
- Unguarded monitors OFF: the chart's `metrics.enabled` stays `false` and no
  `ServiceMonitor` objects are shipped until `monitoring.coreos.com` CRDs land
  (same §9 deviation). Flip: set `metrics.enabled: true` plus
  `metrics.serviceMonitor.enabled: true` once the monitoring stack exists.
- App/chart bumps flow through `update-policies/zitadel.yaml` + PR automation
  (remember the chart↔app divergence above: bump the chart tag AND both image
  tags together).

## Environments

`prd` and `stg` currently inherit `../base` unchanged (same shape
as cert-manager before per-env divergence). Per-env tuning (replica count,
ExternalDomain hostnames, storage size) lands with the first real divergence,
not here.
