# Zitadel

Zitadel app v4.18.0 (chart 10.0.4,
`oci://ghcr.io/zitadel/zitadel-charts/zitadel`): the OIDC issuer on
`https://admin.zitadel.home-ops.yansyah.my.id` (login UI on
`https://login.zitadel.home-ops.yansyah.my.id`, NetBird-exposed) plus the
locked client contract below.

Chart 10.0.4 embeds app v4.15.3; the app image is pinned separately in
values (`image.tag` + `login.image.tag` = v4.18.0). On chart bumps set both
tags to the new chart's appVersion together. Policy floor `>=10.0.4`
(marker `infra:zitadel:tag`).

## Layout

`controllers/{base,dev,prd}` (OCIRepository + HelmRelease with the
`FirstInstance` zero-UI bootstrap stanza; env overlays inherit base
unchanged) and `configs/{base,dev,prd}` (secrets, DB, cache, certificate,
routes, identity intent + bootstrap handoff + COSI claims).

## Dependencies

- Database: `configs/base/zitadel-db.yaml` -- namespace-local CNPG Cluster
  (3 instances, sync quorum 1, `local-ssd-nvme`, continuous WAL + daily base
  backup to SeaweedFS S3 under `s3://cnpg-backups/zitadel/`). dbname/owner
  `zitadel`. Connection via DSN (`ZITADEL_DATABASE_POSTGRES_DSN` from the
  `zitadel-db-credentials` ExternalSecret, `sslmode=require`).
- Cache: `configs/base/zitadel-cache.yaml` -- namespace-local Dragonfly (3
  replicas, tiered persistence, hourly S3 snapshots under
  `s3://dragonfly-backups/zitadel-cache/`). Host
  `zitadel-cache.zitadel.svc.cluster.local`, port 6379, no auth.
- Routing: `configs/base/zitadel-httproute.yaml` -- two HTTPRoutes on the
  shared `Gateway/main` (cross-namespace parentRef): `/` -> `zitadel`
  (8080, h2c) and `/ui/v2/login` -> `zitadel-login` (3000). Chart-native
  ingress/gateway templating stays off. TLS terminates at the Gateway via
  the in-namespace wildcard `Certificate` (namespace-local Secrets, same
  duplicate pattern as gateway-api). The console needs end-to-end HTTP/2
  (Service advertises `appProtocol: kubernetes.io/h2c`).

## Credentials

All secrets sync from Proton Pass via ESO (Git holds `remoteRef`s only):

- `pass://<cluster>/zitadel/masterkey` -- 32-byte masterkey
  (`tr -dc A-Za-z0-9 </dev/urandom | head -c 32`). Immutable: loss means
  loss of all encrypted data.
- `pass://<cluster>/zitadel/db-password` -- single password source: the
  `zitadel-db-credentials` DSN is composed in ESO target.template from
  literal parts plus this field, and `zitadel-db-app-secret` consumes the
  same field (rotate in one place).
- `pass://<cluster>/zitadel/smtp-*` -- relay user/password (optional;
  unwired until a relay exists).
- S3 keys are COSI-minted, not Proton Pass: `cnpg-s3-credentials`
  (zitadel-db) from `zitadel-db-cosi-creds`, `dragonfly-s3-credentials`
  (zitadel-cache) from `zitadel-cache-cosi-creds`,
  `zitadel-asset-storage` (avatars/org logos via `ZITADEL_ASSETSTORAGE_*`)
  from `zitadel-assets-cosi-creds`, all through the in-namespace
  `zitadel-cosi` SecretStore (dedicated claims `zitadel-db` /
  `zitadel-cache` / `zitadel-assets`).
- `pass://<cluster>/cert-manager/cloudflare-api-token` -- same vault path
  as cert-manager, copied so the DNS-01 secret exists in this namespace.

## OIDC contract (locked for app writers)

| Item | Value |
|---|---|
| Issuer (admin host) | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Login UI (NetBird) | `https://login.zitadel.home-ops.yansyah.my.id/ui/v2/login` (per-app `login_base_uri` + instance `LoginV2.BaseURI`; trusted domain registered by the terraform root) |
| Org | `home-ops` |
| Users | `admin@home-ops.yansyah.my.id` (super-admin, `admin` group + role -- bootstrap-owned); non-admin users owned per consumer app |
| Groups | `admin` (admin@ member) -- asserted in the `groups` claim; per-app `users` membership owned by each consumer app |
| Scopes (all clients) | `openid profile email groups` |
| Flow (all clients) | Authorization code + PKCE, refresh tokens on |
| Owner: `coder` -> client `coder` | `https://coder.home-ops.yansyah.my.id/*` (post-logout -> `https://coder.home-ops.yansyah.my.id/`) |
| Owner: `clickstack` -> client `clickstack` | `https://clickstack.home-ops.yansyah.my.id/*` (covers `/oauth2/callback`) |
| Owner: `hubble-ui` -> client `hubble` | `https://hubble.home-ops.yansyah.my.id/*` (covers `/oauth2/callback`) |
| Owner: `flux-operator-ui` -> client `flux-operator-ui` | `https://flux-operator.home-ops.yansyah.my.id/*` (covers `/oauth2/callback`) |
| Owner: `headlamp` -> client `headlamp` | `https://headlamp.home-ops.yansyah.my.id/*` |
| Owner: `seaweedfs` -> client `seaweedfs` | `https://admin.seaweedfs.home-ops.yansyah.my.id/oauth2/callback` (filer-UI proxy) |

Each app owns its own `zitadel_project` + `zitadel_application_oidc` client
in its per-app `terraform/` slice; the central bootstrap owns no clients.
Post-logout redirects point at each app's root.

## Identity bootstrap (Helm FirstInstance, zero-UI)

No Tofu Controller for initial setup -- the chart's setup Job does it:

- `FirstInstance.Org` -- `Name: home-ops` plus the IAM_OWNER machine user
  `zitadel-bootstrap-sa` (`MachineKey` Type 1 JSON + `Pat`, both
  non-expiring by design). The setup Job mints the key JSON + PAT and
  writes kept Secrets `zitadel-bootstrap-sa`
  (`zitadel-bootstrap-sa.json`) and `zitadel-bootstrap-sa-pat` (`pat`);
  `cleanupJob.enabled: false` so both survive reinstalls. No
  `MachineKeyPath`/`PatPath` overrides, no `Org.Skip`, login RSA stays
  chart-managed.
- Both `MachineKey.ExpirationDate` and `Pat.ExpirationDate` are explicit
  `null`, so keys/PATs never expire. `Pat` carries `Scopes: []` so the
  chart template keeps PAT wiring rendered.
- `configs/base/zitadel-bootstrap-handoff.yaml` -- ESO mirrors (no inline
  credentials, no vault seeding):
  - `zitadel-bootstrap-credentials` (`jwt_profile_json`, `pat`) via the
    in-namespace `zitadel-bootstrap` SecretStore (SA
    `eso-zitadel-reader`, get/list/watch on the two setup-Job Secrets).
    Per-app slices consume `jwt_profile_json` via same-namespace `varsFrom`
    or `fileMappings`; app namespaces add a narrow cross-namespace
    Role/RoleBinding (see any `zitadel-handoff-rbac.yaml`).
  - `zitadel-asset-storage` (`endpoint`, `accessKeyId`, `secretAccessKey`)
    via the in-namespace `zitadel-cosi` SecretStore from the colocated
    `zitadel-assets` claim -- consumed as `ZITADEL_ASSETSTORAGE_*` env.
- `configs/base/org-users.yaml` (`zitadel-identity-intent` ConfigMap) --
  human-readable mirror of the intent (kept in sync with the FirstInstance
  stanza on contract changes).
- Human admin (`admin@...`) is operator-invited post-install via the console
  (invite/reset flow) -- not terraform-managed. Non-admin users + OIDC
  clients live in each app's own per-app `terraform/` slice.

Consumer handoff (namespace `zitadel`):

| Secret | Keys | Producer | Consumers use |
|---|---|---|---|
| `zitadel-bootstrap-sa` | `zitadel-bootstrap-sa.json` | chart setup Job (kept) | via `zitadel-bootstrap-credentials` mirror, not directly |
| `zitadel-bootstrap-sa-pat` | `pat` | chart setup Job (kept) | via `zitadel-bootstrap-credentials` mirror, not directly |
| `zitadel-bootstrap-credentials` | `jwt_profile_json`, `pat` | ESO ExternalSecret (`zitadel-bootstrap` store) | tofu `varsFrom`/`fileMappings` |
| `zitadel-bootstrap-outputs` | `org_id`, `admin_user_id` | operator-created once post-install (see below) | per-app CR `vars` (literal, non-sensitive) |
| `zitadel-asset-storage` | `endpoint`, `accessKeyId`, `secretAccessKey` | ESO ExternalSecret (`zitadel-cosi` store) | `ZITADEL_ASSETSTORAGE_*` env |

First-install runbook:

1. Flux applies controllers -> setup Job creates org + machine user + kept
   Secrets (retries until ESO masterkey/DSN Secrets sync).
2. ESO mirrors `zitadel-bootstrap-credentials` + `zitadel-asset-storage`
   (retries until setup-Job Secrets / COSI BucketInfo exist).
3. Operator creates once: `kubectl -n zitadel create secret generic
   zitadel-bootstrap-outputs --from-literal=org_id=<id>
   --from-literal=admin_user_id=<id>`. Invite `admin@...` via the console.

Generated credentials are minted in-cluster and stay in-cluster (chart kept
Secrets -> `zitadel-bootstrap` Kubernetes-provider SecretStore -> ESO
mirrors). Proton Pass holds only static secrets. No `terraform/`
bootstrap slice ships: org/users/membership are FirstInstance-owned;
per-app projects/roles/grants/clients live in each app's own slice. Client
secrets live in each app's per-app state -- read them into Proton Pass,
never Git.

## AssetStorage (S3-backed)

Upstream default is `AssetStorage.Type: db` (avatars/org logos in Postgres).
This component sets `s3` via `ZITADEL_ASSETSTORAGE_*` env vars
(`ZITADEL_` prefix, dots -> underscores): `TYPE=s3`, `ENDPOINT` (internal
SeaweedFS S3, `SSL=false`), `ACCESSKEYID`/`SECRETACCESSKEY` (COSI-minted),
`LOCATION` (`us-east-1`), `BUCKETPREFIX=zitadel-assets`. S3 auto-creates
per-instance buckets (`<prefix>-<instanceID>`); the dedicated
`zitadel-assets` claim never shares the CNPG/Dragonfly backup buckets.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | API HPA 1-2 (seed 1), login HPA 1-2 (seed 1), DB 1, cache 1 | domain + LoginV2 BaseURI, S3 endpoints, vault refs, hostnames, intent mirror + seeds -> 1 + HPAs 1 / 2 + DB `instances` -> 1, cache `replicas` -> 1 |
| `prd` | API HPA 2-4 (seed 2), login HPA 2-4 (seed 2), DB 3, cache 3 | domain + LoginV2 BaseURI, S3 endpoints, vault refs, hostnames, intent mirror + seeds -> 2 + HPAs 2 / 4 + DB `instances` -> 3, cache `replicas` -> 3 |

Rclone sync legs (`zitadel-db` / `zitadel-cache` / `zitadel-assets`): 1 per
instance/schedule, `concurrencyPolicy: Forbid` -- no scaling.

## Telemetry / monitoring / updates

No phone-home knobs in chart values. `metrics.enabled: false`, no
`ServiceMonitor` until `monitoring.coreos.com` CRDs land. Bumps:
`update-policies/zitadel.yaml` -> PR automation (chart tag + both image
tags together). Snapshot DB + cache before major bumps (`masterkey`
immutable, never rotate on upgrade).
Changelogs: https://github.com/zitadel/zitadel-charts/releases,
https://github.com/zitadel/zitadel/releases.
