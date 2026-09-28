# Zitadel

Zitadel app v4.18.0 (chart 10.0.4, `oci://ghcr.io/zitadel/zitadel-charts/zitadel`): the OIDC issuer on `https://admin.zitadel.home-ops.yansyah.my.id` (login UI on `https://login.zitadel.home-ops.yansyah.my.id`, NetBird-exposed) plus the locked client contract below.

Chart/app lockstep (chart 10.0.4 embeds app v4.15.3; app pinned v4.18.0 via `image.tag` + `login.image.tag`): bump checklist in `update-policies/zitadel.yaml`.

## Layout

`controllers/{base,dev,prd}` (OCIRepository + HelmRelease with the `FirstInstance` zero-UI bootstrap stanza; env overlays inherit base unchanged) and `configs/{base,dev,prd}` (secrets, DB, cache, certificate, routes, identity intent + bootstrap handoff + COSI claims).

## Dependencies

- Database: `configs/base/zitadel-db.yaml` — namespace-local CNPG Cluster (3 instances, sync quorum 1, `local-ssd-nvme`, continuous WAL + daily base backup to SeaweedFS S3). dbname/owner `zitadel`. Connection via DSN (`ZITADEL_DATABASE_POSTGRES_DSN`, `sslmode=require` against the operator-managed CA).
- Cache: `configs/base/zitadel-cache.yaml` — namespace-local Dragonfly (3 replicas, tiered persistence, hourly S3 snapshots, AUTH from the ESO-synced `zitadel-cache-auth` Secret). Host `zitadel-cache.zitadel.svc.cluster.local`, port 6379.
- Routing: `configs/base/zitadel-httproute.yaml` — two HTTPRoutes on the shared `Gateway/main` (cross-namespace parentRef): `/` -> `zitadel` (8080, h2c) and `/ui/v2/login` -> `zitadel-login` (3000). Service advertises `appProtocol: kubernetes.io/h2c` (console needs end-to-end HTTP/2). TLS terminates at the Gateway via the in-namespace wildcard `Certificate`.

## Credentials

All secrets sync from Proton Pass via ESO (Git holds `remoteRef`s only): `pass://<cluster>/zitadel/masterkey` (32-byte, immutable — loss means loss of all encrypted data), `pass://<cluster>/zitadel/db-password`, `pass://<cluster>/zitadel/smtp-*` (unwired until a relay exists), `pass://<cluster>/zitadel/cache-password`. Rotating credentials sit behind Reloader (`reloader.stakater.com/auto: "true"` on the `zitadel` + `zitadel-login` Deployments). S3 keys are COSI-minted (claims `zitadel-db` / `zitadel-cache` / `zitadel-assets` through the in-namespace `zitadel-cosi` SecretStore). Terraform `varsFrom` Secrets re-read on each runner reconcile (no Reloader on `kind: Terraform` CRs — rotation lands on the next 30m reconcile). ESO's own `proton-pass-pat` is managed via Talos Ansible.

## OIDC contract (locked for app writers)

| Item | Value |
|---|---|
| Issuer (admin host) | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Login UI (NetBird) | `https://login.zitadel.home-ops.yansyah.my.id/ui/v2/login` |
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

Each app owns its own `zitadel_project` + `zitadel_application_oidc` client in its per-app `terraform/` slice; the central bootstrap owns no clients. Post-logout redirects point at each app's root.

## Identity bootstrap (Helm FirstInstance, zero-UI)

No Tofu Controller for initial setup — the chart's setup Job creates the `home-ops` org + IAM_OWNER machine user (`zitadel-bootstrap-sa`; `cleanupJob.enabled: false` so both survive reinstalls). Current handoff shape in `configs/base/zitadel-bootstrap-handoff.yaml`: chart kept Secrets -> ESO mirror `zitadel-bootstrap-credentials` (`jwt_profile_json`, `pat`, consumed via same-namespace `varsFrom` or cross-namespace Role/RoleBinding) -> tofu `varsFrom`/`fileMappings`; operator-created `zitadel-bootstrap-outputs` (`org_id`, `admin_user_id`) -> per-app CR `vars`; `zitadel-asset-storage` -> `ZITADEL_ASSETSTORAGE_*` env. Human admin (`admin@...`) is operator-invited post-install via the console; client secrets live in each app's per-app state, mirrored to Proton Pass, never Git. Non-expiring first-install keys rotate to 90-day expiries per the runbook noted in the handoff file header.

## AssetStorage (S3-backed)

Upstream default is `AssetStorage.Type: db`; this component sets `s3` via `ZITADEL_ASSETSTORAGE_*` env (`TYPE=s3`, internal SeaweedFS S3 endpoint with `SSL=false`, COSI-minted keys, `LOCATION=us-east-1`, `BUCKETPREFIX=zitadel-assets`). The dedicated `zitadel-assets` claim never shares the CNPG/Dragonfly backup buckets.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | API HPA 1-2 (seed 1), login HPA 1-2 (seed 1), DB 1, cache 1 | domain + LoginV2 BaseURI, S3 endpoints, vault refs, hostnames, intent mirror + seeds -> 1 |
| `prd` | API HPA 2-4 (seed 2), login HPA 2-4 (seed 2), DB 3, cache 3 | domain + LoginV2 BaseURI, S3 endpoints, vault refs, hostnames, intent mirror + seeds -> 2/3 |

Rclone sync legs (`zitadel-db` / `zitadel-cache` / `zitadel-assets`): 1 per instance/schedule, `concurrencyPolicy: Forbid`.

## Updates

`update-policies/zitadel.yaml` (chart tag + both image tags together: chart `ref.tag` + `infra:zitadel-app:tag` + `infra:zitadel-login:tag`). Snapshot DB + cache before major bumps (`masterkey` immutable, never rotate on upgrade). Changelogs: https://github.com/zitadel/zitadel-charts/releases, https://github.com/zitadel/zitadel/releases.
