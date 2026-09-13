# Zitadel

Zitadel **v4.17.1** identity provider: the OIDC issuer for
`https://zitadel.home-ops.yansyah.my.id` plus the locked client contract the
wave-3c app writers build against (table below — do not deviate).

## Chart source

OCI is the upstream source of truth, verified by pull:

- `oci://ghcr.io/zitadel/zitadel-charts/zitadel`, tag **10.0.4** (digest
  `sha256:9afa657fad65079857339f7d7fd296c73e577f6c8ec4e4a103093be35964b50c`,
  `helm template` + `helm lint` pass locally).
- Chart↔app divergence (same split as the cnpg component, unlike the
  dragonfly component where chart and operator versions agree): chart
  **10.0.4** embeds app **v4.15.3**, but the app image is pinned
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
  `infra:zitadel:tag`, same image update policy contract as the
  cert-manager/cnpg/dragonfly components).

## Layout

Mirrors the cert-manager component file-for-file:
`controllers/{base,dev,prd}` (OCIRepository + HelmRelease with the
`FirstInstance` zero-UI bootstrap stanza, env overlays
inherit base unchanged) and `configs/{base,dev,prd}` (secrets, DB, cache,
certificate, routes, identity intent + bootstrap handoff + COSI claims).

## Dependencies

- Database: `configs/base/zitadel-db.yaml` — namespace-local instantiation of
  the cnpg `cluster-base` template
  (`flux/infra/components/cnpg/configs/base/cluster-base.yaml`; same shape:
  3 instances, sync quorum 1, `local-ssd-nvme`, continuous WAL + daily base
  backup to SeaweedFS S3 under `s3://cnpg-backups/zitadel/`). Adjusted:
  dbname/owner `zitadel`, own S3 prefix. Connection via DSN
  (`ZITADEL_DATABASE_POSTGRES_DSN` from the `zitadel-db-credentials`
  ExternalSecret, `sslmode=require`).
- Cache: `configs/base/zitadel-cache.yaml` — namespace-local instantiation of
  the dragonfly `dragonfly-base` template
  (`flux/infra/components/dragonfly/configs/base/dragonfly-base.yaml`;
  3 replicas, tiered persistence, hourly S3 snapshots under
  `s3://dragonfly-backups/zitadel-cache/`). Connection contract per the
  dragonfly component README (`flux/infra/components/dragonfly/README.md`):
  host `zitadel-cache.zitadel.svc.cluster.local`, port **6379**, no auth.
- Routing: `configs/base/zitadel-httproute.yaml` — two hand-written HTTPRoutes
  on the shared Gateway (`main` in `flux/infra/components/gateway-api/`,
  cross-namespace parentRef): `/` → the `zitadel` Service (8080, h2c) and
  `/ui/v2/login` → `zitadel-login` (3000). Chart-native ingress/gateway
  templating stays off so the gateway-api pattern is owned explicitly. TLS
  terminates at the Gateway via the in-namespace wildcard `Certificate`
  (`wildcard-certificate.yaml`, same duplicate pattern as the gateway-api
  component — cert-manager Secrets are namespace-local).
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
  (zitadel-db) syncs from the `zitadel-db-cosi-creds` BucketInfo JSON,
  `dragonfly-s3-credentials` (zitadel-cache) from
  `zitadel-cache-cosi-creds`, and `zitadel-asset-storage` (avatars/org
  logos via `ZITADEL_ASSETSTORAGE_*` env) from `zitadel-assets-cosi-creds`,
  all through the in-namespace `zitadel-cosi`
  SecretStore (dedicated claims `zitadel-db`/`zitadel-cache`/`zitadel-assets`
  — see `configs/base/bucketclaims.yaml` and the cosi README). The
  `pass://…/{cnpg,dragonfly}/s3-*` vault entries stay seeded as rollback.
- `pass://acme-prd-bdo1-talos-apps-01/cert-manager/cloudflare-api-token` —
  same vault path as the cert-manager component, copied so the DNS-01
  secret exists in this namespace too.

## OIDC contract (LOCKED for wave-3c app writers)

App writers build against THIS table — do not deviate.

| Item | Value |
|---|---|
| Issuer | `https://zitadel.home-ops.yansyah.my.id` |
| Org | `home-ops` |
| Users | `admin@home-ops.yansyah.my.id` (super-admin, `admin` group + role — bootstrap-owned); non-admin users are owned per consumer app, not by this bootstrap |
| Groups | `admin` (admin@ member) — asserted in the `groups` claim; per-app `users` membership is owned by each consumer app |
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

## Identity bootstrap (Helm FirstInstance, zero-UI)

No Tofu Controller for initial setup — the chart's setup Job does it
(`controllers/base/zitadel.yaml`):

- `zitadel.configmapConfig.FirstInstance.Org` — `Name: home-ops` plus the
  IAM_OWNER machine user `zitadel-bootstrap-sa` (`MachineKey` Type 1 JSON +
  `Pat`, both expiring 2029-01-01). The setup Job mints the key JSON + PAT
  and writes kept Secrets `zitadel-bootstrap-sa` (key
  `zitadel-bootstrap-sa.json`) and `zitadel-bootstrap-sa-pat` (key `pat`);
  `cleanupJob.enabled: false` so both survive reinstalls. No
  `MachineKeyPath`/`PatPath` overrides (chart-managed — the template fails
  the render if set), no `Org.Skip` (use `FirstInstance.Skip`), login RSA
  stays chart-managed.
- `configs/base/zitadel-bootstrap-handoff.yaml` — ESO mirrors (all secrets
  from ESO, no inline credentials, no manual vault seeding):
  - `zitadel-bootstrap-credentials` (keys `jwt_profile_json`, `pat`) via the
    in-namespace `zitadel-bootstrap` SecretStore (SA `eso-zitadel-reader`,
    get/list/watch on the two setup-Job Secrets only). Per-app
    tofu-controller slices consume `jwt_profile_json` via same-namespace
    `varsFrom` (zitadel ns) or `fileMappings`; app namespaces add a narrow
    cross-namespace Role + RoleBinding on this Secret (same shape as the old
    `*-terraform-remote-state-reader` grants) — this replaces the old
    `pass://<env-vault>/zitadel/terraform-jwt-profile-json` seeding step.
  - `zitadel-asset-storage` (keys `endpoint`, `accessKeyId`,
    `secretAccessKey`) via the in-namespace `zitadel-cosi` SecretStore from
    the colocated `zitadel-assets` claim — consumed as
    `ZITADEL_ASSETSTORAGE_*` env vars by the HelmRelease.
- `configs/base/org-users.yaml` (`zitadel-identity-intent` ConfigMap) —
  human-readable mirror of the intent (kept in sync with the HelmRelease
  FirstInstance stanza on contract changes).
- Human admin (`admin@…`) is operator-invited post-install via the console
  (invite/reset flow) — NOT terraform-managed. Non-admin users + OIDC
  clients live in each app's own per-app `terraform/` slice (own project
  roles/grants assert the `groups` claim); this bootstrap owns no clients.

Consumer handoff (for the follow-up SSO task — Secret names/namespaces/keys):

| Secret (namespace `zitadel`) | Keys | Producer | Consumers use |
|---|---|---|---|
| `zitadel-bootstrap-sa` | `zitadel-bootstrap-sa.json` (machine-key JSON) | chart setup Job (kept) | read via `zitadel-bootstrap-credentials` mirror, not directly |
| `zitadel-bootstrap-sa-pat` | `pat` | chart setup Job (kept) | read via `zitadel-bootstrap-credentials` mirror, not directly |
| `zitadel-bootstrap-credentials` | `jwt_profile_json`, `pat` | ESO ExternalSecret (`zitadel-bootstrap` store) | tofu `varsFrom`/`fileMappings` (`jwt_profile_json` = provider auth, `pat` = API token) |
| `zitadel-bootstrap-outputs` | `org_id`, `admin_user_id` (plain IDs) | operator-created once post-install (see runbook below) | per-app CR `vars` (literal, non-sensitive); replaces `data.terraform_remote_state.zitadel.outputs.*` |
| `zitadel-asset-storage` | `endpoint`, `accessKeyId`, `secretAccessKey` | ESO ExternalSecret (`zitadel-cosi` store ← `zitadel-assets` claim) | `ZITADEL_ASSETSTORAGE_*` env vars (HelmRelease) |

First-install runbook (zero-UI):

1. Flux applies controllers → HelmRelease setup Job creates org + machine
   user + kept Secrets (retries until ESO masterkey/DSN Secrets sync).
2. ESO mirrors `zitadel-bootstrap-credentials` + `zitadel-asset-storage`
   (retries until the setup-Job Secrets / COSI BucketInfo exist).
3. Operator reads `org_id` once (console/API) and creates the plain Secret:
   `kubectl -n zitadel create secret generic zitadel-bootstrap-outputs
   --from-literal=org_id=<id> --from-literal=admin_user_id=<id>`.
   Invite `admin@…` via the console; rotate the machine key by re-running
   the setup Job (or rotating the two kept Secrets — ESO re-mirrors).

Retired: `configs/base/terraform-bootstrap.yaml` (ExternalSecret
`zitadel-terraform-vars` + Terraform CR `zitadel-bootstrap-identity`) and the
`terraform/` bootstrap slice (`zitadel_org`, `zitadel_human_user`,
`zitadel_org_member`, central project/role/grant; provider `zitadel ~> 3.3`)
are deleted. Per-app `data.terraform_remote_state.zitadel` reads keep
working off the LAST tofu-controller state until that state ages out — the
follow-up task switches them to the table above. Client secrets live in each
app's per-app `terraform/` state — read them into Proton Pass (never Git).

## Telemetry-off / monitoring / updates

- Telemetry evidence: the pulled chart `values.yaml` contains no phone-home,
  analytics, or usage-reporting knobs (checked at authoring time; a grep for
  telem\*/analytic\*/phone-home/usage-report/tracking returns nothing).
- Unguarded monitors OFF: the chart's `metrics.enabled` stays `false` and no
  `ServiceMonitor` objects are shipped until `monitoring.coreos.com` CRDs land
  (same as the cert-manager component). Flip: set `metrics.enabled: true`
  plus
  `metrics.serviceMonitor.enabled: true` once the monitoring stack exists.
- App/chart bumps flow through `update-policies/zitadel.yaml` + PR automation
  (remember the chart↔app divergence above: bump the chart tag AND both image
  tags together).

## AssetStorage (S3-backed, not db-default)

Upstream default is `AssetStorage.Type: db` (avatars/org logos in Postgres —
`cmd/defaults.yaml:512`). This component sets `s3` via
`ZITADEL_ASSETSTORAGE_*` env vars (viper: `ZITADEL_` prefix, dots →
underscores): `TYPE=s3`, `ENDPOINT` (internal SeaweedFS S3, plain HTTP so
`SSL=false`), `ACCESSKEYID`/`SECRETACCESSKEY` (COSI-minted), `LOCATION`
(`us-east-1`, minio region passthrough), `BUCKETPREFIX=zitadel-assets`.
Zitadel's S3 backend auto-creates per-instance buckets
(`<prefix>-<instanceID>`, `PutObject.createBucket`) and never shares the
CNPG/Dragonfly backup buckets — dedicated `zitadel-assets` claim on purpose.

## Environments

`dev` and `prd` each carry per-env patches (external domain, DB/cache/asset
S3 endpoints, vault refs, hostnames, intent mirror); controllers add only the
external-domain patch each. Per-env tuning (replica count,
ExternalDomain hostnames, storage size) lands with the first real divergence,
not here.
