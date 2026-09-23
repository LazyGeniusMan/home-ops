# Zitadel

Zitadel **v4.17.1** identity provider: the OIDC issuer on `https://admin.zitadel.home-ops.yansyah.my.id` (login UI on `https://login.zitadel.home-ops.yansyah.my.id`, NetBird-exposed) plus the locked client contract the
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
  plus the pulled chart `values.yaml`/templates. Chart repo + provider
  contracts: `/tmp/home-ops-docs/zitadel-helm-charts-docs`,
  `/tmp/home-ops-docs/zitadel-terraform-provider-docs`.
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
- `pass://acme-prd-bdo1-talos-apps-01/zitadel/db-password` — SINGLE password
  source: the `zitadel-db-credentials` DSN is composed in ESO target.template
  from literal parts (user/host/port/db + `sslmode=require`) plus this field,
  and `zitadel-db-app-secret` consumes the same field (no dual-write; rotate
  in one place).
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
| Issuer (admin host) | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Login UI (NetBird) | `https://login.zitadel.home-ops.yansyah.my.id/ui/v2/login` (per-app `login_base_uri` + instance `LoginV2.BaseURI`; trusted domain registered by the terraform root) |
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
| Owner: `seaweedfs` → client `seaweedfs` | `https://admin.seaweedfs.home-ops.yansyah.my.id/oauth2/callback` (serves the filer-UI proxy) |

Each app owns its own `zitadel_project` + `zitadel_application_oidc` client
in its per-app `terraform/` slice (own project roles/grants assert the
`groups` claim); the central bootstrap slice owns no clients. Post-logout
redirects point at each app's root (`https://<app>…/`).

## Identity bootstrap (Helm FirstInstance, zero-UI)

No Tofu Controller for initial setup — the chart's setup Job does it
(`controllers/base/zitadel.yaml`):

- `zitadel.configmapConfig.FirstInstance.Org` — `Name: home-ops` plus the
  IAM_OWNER machine user `zitadel-bootstrap-sa` (`MachineKey` Type 1 JSON +
  `Pat`, both NON-EXPIRING by design — see below). The setup Job mints the
  key JSON + PAT
  and writes kept Secrets `zitadel-bootstrap-sa` (key
  `zitadel-bootstrap-sa.json`) and `zitadel-bootstrap-sa-pat` (key `pat`);
  `cleanupJob.enabled: false` so both survive reinstalls. No
  `MachineKeyPath`/`PatPath` overrides (chart-managed — the template fails
  the render if set), no `Org.Skip` (use `FirstInstance.Skip`), login RSA
  stays chart-managed.

Non-expiring bootstrap auth (maintenance-free by design):

- Both `MachineKey.ExpirationDate` and `Pat.ExpirationDate` are explicit
  `null` in `controllers/base/zitadel.yaml` — fresh-install keys/PATs never
  expire, so the six tofu-controller `Terraform` objects (coder, clickstack,
  hubble-ui, flux-operator-ui, headlamp, seaweedfs, all on
  `jwt_profile_json`) authenticate indefinitely with zero maintenance (no
  CronJob, no rotation automation, no provider-mode switch; the PAT stays
  mirrored-idle via `zitadel-bootstrap-credentials`).
- Why explicit null, not omission: Helm coalesce fills an OMITTED key with
  the chart `values.yaml` default (`2029-01-01T00:00:00Z`), verified by
  `helm template` against chart 10.0.4. Explicit `null` overrides the
  default and renders as an absent key (no `ExpirationDate` line in the
  Zitadel ConfigMap, no `2029` anywhere in the render); schema
  (`values.schema.json`) requires nothing under `MachineKey`/`Pat`, and
  `helm lint` passes. `Pat` also carries `Scopes: []` so the chart
  template's `$hasMachinePat` stays truthy (PATPATH env + pat-writer
  sidecar render); a bare `Pat: {}` would silently drop PAT wiring.
- Why absent means never-expires (server side): the setup Job feeds the
  ConfigMap through viper/mapstructure into `time.Time`; an absent key
  decodes to the zero time, and `internal/domain/expiration.go`
  `ValidateExpirationDate` maps zero → `9999-12-31T23:59:59Z` for both
  machine keys and PATs (`user_machine_key.go` / `user_personal_access_token.go`
  call it from `valid()`). This is the config-file equivalent of the guides'
  "leave empty for no expiration" (private-key-jwt guide: "Optionally set an
  expiration date for the key, or leave empty for no expiration";
  personal-access-token guide: "You can either set an expiration date or
  leave it empty if you don't want it to expire").
- No in-place extend exists (rotation = create-new + delete-old), so
  no-expiry is the fix — not a rotation CronJob/operator (deliberately no
  new moving parts).

One-time migration for 2029-expiry keys (FirstInstance only runs on
fresh setup — changing values does NOT re-mint keys on existing installs):

1. Delete the kept Secrets so the setup Job re-runs key creation:
   `kubectl -n zitadel delete secret zitadel-bootstrap-sa
   zitadel-bootstrap-sa-pat` (both carry `helm.sh/resource-policy=keep`, so
   delete explicitly), then restart/re-run the `zitadel-setup` Job. ESO
   re-mirrors `zitadel-bootstrap-credentials` automatically
   (`refreshInterval: 1h`).
2. Alternative: rotate via console/API (create a new non-expiring key/PAT
   on `zitadel-bootstrap-sa`, update the kept Secrets, delete the old ones).
3. Verify: new key JSON / PAT carry no expiry (server shows 9999-12-31 or
   empty); `grep -rn 2029` under this component is empty.
- `configs/base/zitadel-bootstrap-handoff.yaml` — ESO mirrors (all secrets
  from ESO, no inline credentials, no manual vault seeding):
  - `zitadel-bootstrap-credentials` (keys `jwt_profile_json`, `pat`) via the
    in-namespace `zitadel-bootstrap` SecretStore (SA `eso-zitadel-reader`,
    get/list/watch on the two setup-Job Secrets only). Per-app
    tofu-controller slices consume `jwt_profile_json` via same-namespace
    `varsFrom` (zitadel ns) or `fileMappings`; app namespaces add a narrow
    cross-namespace Role + RoleBinding on this Secret (see any
    `zitadel-handoff-rbac.yaml` consumer). Provider auth never lives in the
    vault — no `pass://` seeding step exists.
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

Consumer handoff (Secret names/namespaces/keys consumed by each app's SSO Terraform CR):

| Secret (namespace `zitadel`) | Keys | Producer | Consumers use |
|---|---|---|---|
| `zitadel-bootstrap-sa` | `zitadel-bootstrap-sa.json` (machine-key JSON) | chart setup Job (kept) | read via `zitadel-bootstrap-credentials` mirror, not directly |
| `zitadel-bootstrap-sa-pat` | `pat` | chart setup Job (kept) | read via `zitadel-bootstrap-credentials` mirror, not directly |
| `zitadel-bootstrap-credentials` | `jwt_profile_json`, `pat` | ESO ExternalSecret (`zitadel-bootstrap` store) | tofu `varsFrom`/`fileMappings` (`jwt_profile_json` = provider auth, `pat` = API token) |
| `zitadel-bootstrap-outputs` | `org_id`, `admin_user_id` (plain IDs) | operator-created once post-install (see runbook below) | per-app CR `vars` (literal, non-sensitive) |
| `zitadel-asset-storage` | `endpoint`, `accessKeyId`, `secretAccessKey` | ESO ExternalSecret (`zitadel-cosi` store ← `zitadel-assets` claim) | `ZITADEL_ASSETSTORAGE_*` env vars (HelmRelease) |

First-install runbook (zero-UI):

1. Flux applies controllers → HelmRelease setup Job creates org + machine
   user + kept Secrets (retries until ESO masterkey/DSN Secrets sync).
2. ESO mirrors `zitadel-bootstrap-credentials` + `zitadel-asset-storage`
   (retries until the setup-Job Secrets / COSI BucketInfo exist).
3. Operator reads `org_id` once (console/API) and creates the plain Secret:
   `kubectl -n zitadel create secret generic zitadel-bootstrap-outputs
   --from-literal=org_id=<id> --from-literal=admin_user_id=<id>`.
   Invite `admin@…` via the console. Fresh installs mint non-expiring keys
   (nothing to rotate). For pre-existing 2029-expiry keys see the one-time
   migration above (delete the two kept Secrets + re-run the setup Job, or
   rotate via console/API — ESO re-mirrors).

ESO pull-only is sufficient (no push needed):

- Generated credentials (machine-key JSON, PAT) are minted in-cluster by
  the chart setup Job and stay in-cluster: chart kept Secrets →
  `zitadel-bootstrap` Kubernetes-provider SecretStore → ESO ExternalSecret
  mirrors (`zitadel-bootstrap-credentials`). Proton Pass holds only STATIC
  secrets (masterkey, DSN, passwords); nothing generated ever flows back to
  the vault.
- `projects/eso-proton-pass` is pull-only by design (`POST /push` → 501,
  `Push() → ErrPushUnimplemented`); the repo carries zero `kind:
  PushSecret`. That path is never exercised by this component — no code
  changes, no PushSecret, no push wiring needed.

No `terraform/` bootstrap slice ships: org, users, and membership are owned
by the FirstInstance stanza above; per-app projects/roles/grants/clients live
in each app's own `terraform/` slice (see any app README). Client secrets
live in each app's per-app `terraform/` state — read them into Proton Pass
(never Git).

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

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | API 2 (base value, single-node scale), login 1, DB 1, cache 1 (stateful legs single-instance, no quorum) | external domain, DB/cache/asset S3 endpoints, vault refs, hostnames, intent mirror + DB `instances` → 1, cache `replicas` → 1 |
| `prd` | API 2, login 1, DB 3, cache 3 (recommended production) | external domain, DB/cache/asset S3 endpoints, vault refs, hostnames, intent mirror + DB `instances` → 3, cache `replicas` → 3 |

Controllers add only the external-domain patch each (base chart values
already pin `replicaCount: 2` API / `1` login — the production shape;
`dev` runs them as-is at single-node scale).
Rclone sync (`zitadel-db` / `zitadel-cache` / `zitadel-assets` legs):
1 per instance/schedule, `concurrencyPolicy: Forbid` — no scaling.

Upstream reference (read-only): `/tmp/home-ops-docs/zitadel-docs`.
