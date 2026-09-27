# matrix

Single-tenant Matrix stack: one namespace (`matrix`), one OCI artifact
(`apps/matrix`), one Fleet Kustomization. Holds the `tuwunel` homeserver,
the `mautrix-discord` bridge (+ colocated Postgres), the `element-web` SPA,
and consumer-only apprise wiring (the `apprise-go-api` workload lives in the
infra `apprise-go-api` tenant as shared credential-free platform plumbing).

## Workloads

| Workload | Role | Image | Scaling |
|---|---|---|---|
| `apprise-go-api` (consumer only) | NO workload here — sink lives in the infra tenant. This tenant keeps the `apprise-stateless-urls` fallback ES + Provider/Alert wiring, posting to `http://apprise-go-api.apprise-go-api.svc:80/notify` | n/a (policy `infra:apprise-go-api:tag`) | n/a (infra HPA 1–2 dev / 2–4 prd, VPA Off) |
| `tuwunel` | Matrix homeserver (`server_name == tuwunel.matrix.<env>`, Zitadel SSO, federation off, RocksDB on S3-backed media) | `ghcr.io/matrix-construct/tuwunel` (`$imagepolicy` → `apps:tuwunel:tag`) | Singleton (no HPA), VPA Initial, PDB n/a (1 replica) |
| `mautrix-discord` | Discord puppeting bridge (`@discordbot:<server>`) + colocated `mautrix-discord-db` CNPG Cluster | `dock.mau.dev/mautrix/discord:v0.7.7` (pinned, no policy) | Both singleton (bridge 1, DB 1 dev / 3 prd), VPA Initial, PDB n/a (1 replica) |
| `element-web` | Public stateless SPA speaking to tuwunel (Gateway + NetBird) | `vectorim/element-web` (`$imagepolicy` → `apps:element-web:tag`) | HPA 1–2 dev / 2–4 prd, VPA Off, PDB minAvailable 1 |

## Layout

`base/` holds every manifest; env overlays `{dev,prd}/` patch hostnames,
vault refs, identity values, and replica bounds via
`resources: [../base]` + RFC-6902 patches, grouped under per-workload
`---- <workload> ----` section headers.

## Shared files (one copy serves the whole tenant)

- `wildcard-certificate.yaml`: TWO Certificates (one wildcard = one label):
  `__WILDCARD_CERT_NAME__` for `*.__BASE_DOMAIN__` + `matrix-nested` for
  `*.matrix.__BASE_DOMAIN__` (covers `tuwunel.matrix.*` +
  `element.matrix.*`), both via ClusterIssuer/letsencrypt. Edge TLS
  terminates at the shared `main` Gateway — its wildcard Certificate (in the
  gateway-api component) must carry the same `*.matrix.<base>` SAN.
- `tuwunel-storage.yaml` + `mautrix-storage.yaml`: SEPARATE
  (tuwunel app media vs CNPG WAL+base backups). Same split for
  `tuwunel-cosi-keys.yaml` + `mautrix-cosi-keys.yaml` (distinct
  SecretStores). All `remoteNamespace:` values are `matrix`.
- `matrix-rbac.yaml`: ONE shared `eso-k8s-reader`
  ServiceAccount/Role/Binding serving every in-namespace k8s store
  (`tuwunel-k8s`, `tuwunel-cosi`, `mautrix-discord-cosi`).
- `cloudflare-api-token`: ONE ExternalSecret (inside
  `tuwunel-secrets.yaml`).
- `notifications.yaml` + apprise secrets are consumer-only wiring
  (Provider addresses are the infra Service DNS
  `http://apprise-go-api.apprise-go-api.svc:80/…`).
  `apprise-sink-allowlist.yaml` is the REFERENCE-ONLY sink fence
  (default-deny + notifier allowlist for namespace `apprise-go-api`; NOT in
  `base/kustomization.yaml` — the sink lives in the infra tenant, so the sink
  owner copies the rules there when that tenant gains a configs overlay).

## Image policies

Markers reference policy NAMES (`apps:element-web:tag`,
`apps:tuwunel:tag`), not paths. mautrix-discord (v0.7.7) stays pinned,
no policy: upstream is `dock.mau.dev` (manual bumps, see
`base/mautrix-discord.yaml`).

## Per-workload docs

`projects/apprise-go-api/README.md`, `base/NOTIFICATIONS.md`
(Flux→apprise→Matrix wiring + tag/matrix contract).

## First-login runbook (designated admin) — FLIPPED 2026-09-27

`tuwunel` ships with `TUWUNEL_GRANT_ADMIN_TO_FIRST_USER=false` (staged
kill-switch safe default — admin state lives in the homeserver DB and is
repo-unverifiable, so the flip ships with the safe default). First-login
admin claims were consumed per the runbook below (one designated human
admin per env, exactly once):

| Env | Designated admin (signed in FIRST) | Server |
| --- | --- | --- |
| `dev` | `@admin:tuwunel.matrix.home-ops-dev.yansyah.my.id` | `tuwunel.matrix.home-ops-dev.yansyah.my.id` |
| `prd` | `@admin:tuwunel.matrix.home-ops.yansyah.my.id` | `tuwunel.matrix.home-ops.yansyah.my.id` |

Completed steps (kept for re-bootstrap):

1. Ensure the Zitadel `tuwunel` user exists (invite/reset flow — never commit passwords).
2. ~~Confirm `TUWUNEL_GRANT_ADMIN_TO_FIRST_USER=true` is still in `base/tuwunel.yaml`~~ — DONE, base now ships `false`.
3. Sign in via Element (`element.matrix.<env>`) with SSO as the designated admin FIRST.
4. Verify admin (admin room created, `CREATE_ADMIN_ROOM=true`).
5. ~~Post-bootstrap: set `TUWUNEL_GRANT_ADMIN_TO_FIRST_USER=false`~~ — DONE (2026-09-27; later sign-ins stay unprivileged).
5. The bridge admin (`BRIDGE_ADMIN_MXID`, same `@admin` MXID) + room leads (`@oncall-lead`, `@coder-admin` in rooms overlays) are separate grants — they ride the Terraform room CRs, not this flag.

Re-bootstrap (fresh DB only): flip to `true` locally (never commit),
sign in, verify, flip back.

## Probes

Fixed-path exceptions (no `/healthz` + `/readyz` on these images — each
carries a why-comment at its probe block): tuwunel probes exec
`["tuwunel", "--health-check"]` (no HTTP health endpoint); mautrix-discord
probes are TCP sockets on the appservice listener (`:29334`, no health
endpoint at this revision); element-web probes hit `/` on `:80` (nginx SPA —
no dedicated health path).

## Backups (bare-minimum leg)

Every file-based singleton has a periodic leg to the in-cluster SeaweedFS S3
bucket, then rclone offsite to Proton Drive (`home-ops/backups/<cluster>/matrix/`):

| Leg | Schedule | Direction |
| --- | --- | --- |
| `rclone-sync-tuwunel-data` | 02:30 | PVC `tuwunel-data` → S3 `tuwunel-media/` (live RocksDB copy) |
| `rclone-sync-mautrix-discord-data` | 02:45 | PVC `mautrix-discord-data` → S3 `tuwunel-media/` (config + registration) |
| `rclone-sync-tuwunel-media` | 05:00 | S3 `tuwunel-media/` → Proton Drive (media + PVC copies offsite) |
| `rclone-sync-mautrix-discord-db` | 05:30 | S3 `cnpg-backups/mautrix-discord/` → Proton Drive (CNPG WAL+base offsite) |

All four carry `activeDeadlineSeconds: 3600`, `concurrencyPolicy: Forbid`.
Restore: scale tuwunel to 0, sync the S3 prefix back into the PVC, scale to 1
(`server_name` IMMUTABLE — a rename is a fresh install, not a restore).
`mautrix-discord-db` additionally has continuous CNPG WAL + daily base backups
(`retentionPolicy: 30d`).

## Secret rotation

All rotating creds arrive via ESO (hourly refresh) and every Deployment/Job
carries `reloader.stakater.com/auto: "true"` (infra reloader component rolls
pods when mounted Secrets/ConfigMaps change — safe no-op until it exists).
Rotate by changing the vault field (or deleting `registration.yaml` for the
bridge re-register flow); no pod restarts by hand.

## Updates

Version sources: image tags in `base/tuwunel.yaml` (auto via
`apps:tuwunel:tag`), `base/element-web.yaml` (auto via
`apps:element-web:tag`), and `base/mautrix-discord.yaml` (bridge
v0.7.7, PINNED — no policy, upstream is `dock.mau.dev`). ImagePolicy PRs land
for tuwunel/element-web; the bridge pin moves by hand after reading its release
notes (config shape is authored from `example-config.yaml` — re-diff on
bumps). tuwunel is a singleton on a single RWO PVC (RocksDB single-writer,
`Recreate`) and `server_name` is IMMUTABLE — confirm the backup legs above
are green + snapshot the `mautrix-discord-db` CNPG cluster before major bumps.
Changelogs: tuwunel https://github.com/matrix-construct/tuwunel/releases ·
element https://github.com/element-hq/element-web/releases · bridge
https://github.com/mautrix/discord/releases.

## Environments

| Env | Hosts | Notable patches |
| --- | --- | --- |
| `dev` | `tuwunel.matrix.home-ops-dev.yansyah.my.id`, `element.matrix.home-ops-dev.yansyah.my.id`, Zitadel `admin.zitadel.home-ops-dev.yansyah.my.id` | vault refs, wildcard certs (base + nested `*.matrix`), hostnames, SERVER_NAME + well-known + issuer + callback, bridge HS link + HS_DOMAIN/admin MXID, element config.json, proxy vars, HPA bounds, DB instances 1 |
| `prd` | `tuwunel.matrix.home-ops.yansyah.my.id`, `element.matrix.home-ops.yansyah.my.id`, Zitadel `admin.zitadel.home-ops.yansyah.my.id` | same shape, DB instances 3, HPA floors 2 |

## Verification

```bash
kustomize build flux/apps/components/matrix/{dev,prd} --load-restrictor=LoadRestrictionsNone | kubeconform -strict -ignore-missing-schemas
./flux/scripts/validate.sh -d flux/apps
```
