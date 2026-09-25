# Zitadel

Zitadel app v4.18.0 (chart 10.0.4,
`oci://ghcr.io/zitadel/zitadel-charts/zitadel`): the OIDC issuer on
`https://admin.zitadel.home-ops.yansyah.my.id` (login UI on
`https://login.zitadel.home-ops.yansyah.my.id`, NetBird-exposed) plus the locked
client contract below.

Chart 10.0.4 embeds app v4.15.3; the app image is pinned separately in values
(`image.tag` + `login.image.tag` = v4.18.0). On chart bumps set both tags to the
new chart's appVersion together. Policy floor `>=10.0.4` (marker
`infra:zitadel:tag`).

## Layout

`controllers/{base,dev,prd}` (OCIRepository + HelmRelease with the
`FirstInstance` zero-UI bootstrap stanza; env overlays inherit base unchanged)
and `configs/{base,dev,prd}` (secrets, DB, cache, certificate, routes, identity
intent + bootstrap handoff + COSI claims).

## Dependencies

- Database: `configs/base/zitadel-db.yaml` — namespace-local CNPG Cluster (3
  instances, sync quorum 1, `local-ssd-nvme`, continuous WAL + daily base backup
  to SeaweedFS S3). dbname/owner `zitadel`. Connection via DSN
  (`ZITADEL_DATABASE_POSTGRES_DSN`, `sslmode=require`).
- Cache: `configs/base/zitadel-cache.yaml` — namespace-local Dragonfly (3
  replicas, tiered persistence, hourly S3 snapshots). Host
  `zitadel-cache.zitadel.svc.cluster.local`, port 6379, no auth.
- Routing: `configs/base/zitadel-httproute.yaml` — two HTTPRoutes on the shared
  `Gateway/main` (cross-namespace parentRef): `/` -> `zitadel` (8080, h2c) and
  `/ui/v2/login` -> `zitadel-login` (3000). Chart-native ingress/gateway
  templating stays off. TLS terminates at the Gateway via the in-namespace
  wildcard `Certificate`. The console needs end-to-end HTTP/2 (Service
  advertises `appProtocol: kubernetes.io/h2c`).

## Credentials

All secrets sync from Proton Pass via ESO (Git holds `remoteRef`s only):
`pass://<cluster>/zitadel/masterkey` (32-byte, immutable — loss means loss of
all encrypted data), `pass://<cluster>/zitadel/db-password` (single source for
the DSN and the CNPG app secret — rotate in one place),
`pass://<cluster>/zitadel/smtp-*` (unwired until a relay exists). S3 keys are
COSI-minted, not Proton Pass (claims `zitadel-db` / `zitadel-cache` /
`zitadel-assets` through the in-namespace `zitadel-cosi` SecretStore).
`pass://<cluster>/cert-manager/cloudflare-api-token` mirrors the DNS-01 secret
into this namespace.

## OIDC contract (locked for app writers)

| Item | Value |
|---|---|
| Issuer (admin host) | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Login UI (NetBird) | `https://login.zitadel.home-ops.yansyah.my.id/ui/v2/login` (per-app `login_base_uri` + trusted-domain registration by the terraform root) |
| Org | `home-ops` |
| Users | `admin@home-ops.yansyah.my.id` (super-admin, bootstrap-owned); non-admin users owned per consumer app |
| Groups | `admin` (asserted in the `groups` claim); per-app `users` membership owned by each consumer app |
| Scopes / flow (all clients) | `openid profile email groups` / authorization code + PKCE, refresh tokens on |
| Owner: `coder` -> client `coder` | `https://coder.home-ops.yansyah.my.id/*` |
| Owner: `clickstack` -> client `clickstack` | `https://clickstack.home-ops.yansyah.my.id/*` |
| Owner: `hubble-ui` -> client `hubble` | `https://hubble.home-ops.yansyah.my.id/*` |
| Owner: `flux-operator-ui` -> client `flux-operator-ui` | `https://flux-operator.home-ops.yansyah.my.id/*` |
| Owner: `headlamp` -> client `headlamp` | `https://headlamp.home-ops.yansyah.my.id/*` |
| Owner: `seaweedfs` -> client `seaweedfs` | `https://admin.seaweedfs.home-ops.yansyah.my.id/oauth2/callback` (filer-UI proxy) |

Each app owns its own `zitadel_project` + `zitadel_application_oidc` client in
its per-app `terraform/` slice; the central bootstrap owns no clients.
Post-logout redirects point at each app's root.

## Identity bootstrap (Helm FirstInstance, zero-UI)

No Tofu Controller for initial setup — the chart's setup Job creates the
`home-ops` org + IAM_OWNER machine user (`zitadel-bootstrap-sa`, non-expiring
key JSON + PAT; `cleanupJob.enabled: false` so both survive reinstalls).
`configs/base/zitadel-bootstrap-handoff.yaml` mirrors them via ESO into
`zitadel-bootstrap-credentials` (consumed by per-app slices via same-namespace
`varsFrom` or cross-namespace Role/RoleBinding); the colocated
`zitadel-assets` claim feeds `zitadel-asset-storage` (`ZITADEL_ASSETSTORAGE_*`
env). `configs/base/org-users.yaml` mirrors the intent human-readably (kept in
sync with the FirstInstance stanza). Human admin (`admin@...`) is
operator-invited post-install via the console; client secrets live in each
app's per-app state — read them into Proton Pass, never Git.

Consumer handoff (namespace `zitadel`): `zitadel-bootstrap-sa` /
`zitadel-bootstrap-sa-pat` (chart kept Secrets) -> `zitadel-bootstrap-credentials`
(`jwt_profile_json`, `pat`, ESO mirror) -> tofu `varsFrom`/`fileMappings`;
`zitadel-bootstrap-outputs` (`org_id`, `admin_user_id`) is operator-created once
post-install -> per-app CR `vars`; `zitadel-asset-storage` (`endpoint`,
`accessKeyId`, `secretAccessKey`) -> `ZITADEL_ASSETSTORAGE_*` env.

First install reconciles declaratively: Flux applies controllers -> setup Job
creates org + machine user + kept Secrets -> ESO mirrors credentials ->
operator records the bootstrap-outputs Secret once and invites `admin@...` via
the console. Generated credentials stay in-cluster (chart kept Secrets ->
Kubernetes-provider SecretStore -> ESO mirrors); Proton Pass holds only static
secrets.

## AssetStorage (S3-backed)

Upstream default is `AssetStorage.Type: db`. This component sets `s3` via
`ZITADEL_ASSETSTORAGE_*` env vars (`TYPE=s3`, internal SeaweedFS S3 endpoint
with `SSL=false`, COSI-minted keys, `LOCATION=us-east-1`,
`BUCKETPREFIX=zitadel-assets`). S3 auto-creates per-instance buckets; the
dedicated `zitadel-assets` claim never shares the CNPG/Dragonfly backup
buckets.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | API HPA 1-2 (seed 1), login HPA 1-2 (seed 1), DB 1, cache 1 | domain + LoginV2 BaseURI, S3 endpoints, vault refs, hostnames, intent mirror + seeds -> 1 |
| `prd` | API HPA 2-4 (seed 2), login HPA 2-4 (seed 2), DB 3, cache 3 | domain + LoginV2 BaseURI, S3 endpoints, vault refs, hostnames, intent mirror + seeds -> 2/3 |

Rclone sync legs (`zitadel-db` / `zitadel-cache` / `zitadel-assets`): 1 per
instance/schedule, `concurrencyPolicy: Forbid` — no scaling.

## Telemetry / monitoring / updates

No phone-home knobs in chart values. `metrics.enabled: false`, no
`ServiceMonitor` until `monitoring.coreos.com` CRDs land. Bumps:
`update-policies/zitadel.yaml` -> PR automation (chart tag + both image tags
together). Snapshot DB + cache before major bumps (`masterkey` immutable, never
rotate on upgrade).
Changelogs: https://github.com/zitadel/zitadel-charts/releases,
https://github.com/zitadel/zitadel/releases.
