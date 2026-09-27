# matrix

Single-tenant Matrix stack: one namespace (`matrix`), one OCI artifact (`apps/matrix`), one Fleet Kustomization. Holds the `tuwunel` homeserver, the `mautrix-discord` bridge (+ colocated Postgres), the `element-web` SPA, and consumer-only apprise wiring (the `apprise-go-api` workload lives in the infra tenant).

## Workloads

| Workload | Role | Image | Scaling |
|---|---|---|---|
| `apprise-go-api` (consumer only) | No workload here — fallback ES + Provider/Alert wiring posting to `http://apprise-go-api.apprise-go-api.svc:80/notify` | n/a (policy `infra:apprise-go-api:tag`) | n/a |
| `tuwunel` | Homeserver (`server_name == tuwunel.matrix.<env>`, Zitadel SSO, federation off) | `ghcr.io/matrix-construct/tuwunel` (`$imagepolicy` → `apps:tuwunel:tag`) | Singleton (no HPA), VPA Initial |
| `mautrix-discord` | Discord bridge (`@discordbot:<server>`) + colocated CNPG Cluster | `dock.mau.dev/mautrix/discord:v0.7.7` (pinned, no policy) | Both singleton, VPA Initial |
| `element-web` | Public stateless SPA | `vectorim/element-web` (`$imagepolicy` → `apps:element-web:tag`) | HPA 1–2 dev / 2–4 prd, VPA Off |

## Layout

`base/` holds every manifest; `{dev,prd}/` patch hostnames, vault refs, identity values, and replica bounds. Per-workload docs: `projects/apprise-go-api/README.md`, `base/NOTIFICATIONS.md`.

## Shared files

- `wildcard-certificate.yaml`: two Certificates (one wildcard = one label): `__WILDCARD_CERT_NAME__` for `*.__BASE_DOMAIN__` + `matrix-nested` for `*.matrix.__BASE_DOMAIN__`, both via `ClusterIssuer/letsencrypt`. The `main` Gateway wildcard must carry the same `*.matrix.<base>` SAN.
- `tuwunel-storage.yaml` + `mautrix-storage.yaml`: separate (app media vs CNPG backups); same split for `tuwunel-cosi-keys.yaml` + `mautrix-cosi-keys.yaml`.
- `matrix-rbac.yaml`: one shared `eso-k8s-reader` SA/Role/Binding for every in-namespace k8s store.
- `cloudflare-api-token`: one ExternalSecret (inside `tuwunel-secrets.yaml`).
- `notifications.yaml` + apprise secrets: consumer-only wiring (Provider addresses are the infra Service DNS). `apprise-sink-allowlist.yaml` is reference-only (not in `base/kustomization.yaml`).

## First login

`tuwunel` ships `TUWUNEL_GRANT_ADMIN_TO_FIRST_USER=false`. Designated admin signs in FIRST via Element SSO (`@admin:tuwunel.matrix.<env>`). Re-bootstrap (fresh DB only): flip to `true` locally (never commit), sign in, verify, flip back.

## Probes

Fixed-path exceptions (why-comment at each probe block): tuwunel exec `["tuwunel", "--health-check"]`; mautrix-discord TCP `:29334`; element-web `/` on `:80`.

## Backups

Every file-based singleton has a periodic leg to in-cluster SeaweedFS S3, then rclone offsite to Proton Drive (`home-ops/backups/<cluster>/matrix/`):

| Leg | Schedule | Direction |
| --- | --- | --- |
| `rclone-sync-tuwunel-data` | 02:30 | PVC `tuwunel-data` → S3 `tuwunel-media/` |
| `rclone-sync-mautrix-discord-data` | 02:45 | PVC `mautrix-discord-data` → S3 `tuwunel-media/` |
| `rclone-sync-tuwunel-media` | 05:00 | S3 `tuwunel-media/` → Proton Drive |
| `rclone-sync-mautrix-discord-db` | 05:30 | S3 `cnpg-backups/mautrix-discord/` → Proton Drive |

Restore: scale tuwunel to 0, sync the S3 prefix back into the PVC, scale to 1 (`server_name` is IMMUTABLE — a rename is a fresh install). `mautrix-discord-db` also has CNPG WAL + daily base backups (`retentionPolicy: 30d`).

## Updates

Image tags: `base/tuwunel.yaml` (auto), `base/element-web.yaml` (auto), `base/mautrix-discord.yaml` (bridge v0.7.7, PINNED — re-diff from `example-config.yaml` on bumps). Snapshot the CNPG cluster before major bumps. Changelogs: [tuwunel](https://github.com/matrix-construct/tuwunel/releases) · [element-web](https://github.com/element-hq/element-web/releases) · [bridge](https://github.com/mautrix/discord/releases).

## Environments

| Env | Hosts | Notable patches |
| --- | --- | --- |
| `dev` | `tuwunel.matrix.home-ops-dev.yansyah.my.id`, `element.matrix.home-ops-dev.yansyah.my.id` | vault refs, certs, hostnames, HPA bounds, DB 1 |
| `prd` | `tuwunel.matrix.home-ops.yansyah.my.id`, `element.matrix.home-ops.yansyah.my.id` | same shape, DB 3, HPA floors 2 |
