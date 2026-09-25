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
| `tuwunel` | Matrix homeserver (`server_name == tuwunel.matrix.<env>`, Zitadel SSO, federation off, RocksDB on S3-backed media) | `ghcr.io/matrix-construct/tuwunel` (`$imagepolicy` → `apps:tuwunel:tag`) | Singleton (no HPA), VPA Auto |
| `mautrix-discord` | Discord puppeting bridge (`@discordbot:<server>`) + colocated `mautrix-discord-db` CNPG Cluster | `dock.mau.dev/mautrix/discord:v0.7.7` (pinned, no policy) | Both singleton (bridge 1, DB 1 dev / 3 prd), VPA Initial |
| `element-web` | Public stateless SPA speaking to tuwunel (Gateway + NetBird) | `vectorim/element-web` (`$imagepolicy` → `apps:element-web:tag`) | HPA 1–2 dev / 2–4 prd, VPA Off |

## Layout

`base/` holds every manifest; env overlays `{dev,prd}/` patch hostnames,
vault refs, identity values, and replica bounds via
`resources: [../base]` + RFC-6902 patches, grouped under per-workload
`---- <workload> ----` section headers.

## Shared files (one copy serves the whole tenant)

- `wildcard-certificate.yaml`: ONE copy covering both nested hostnames
  (`tuwunel.matrix.*` + `element.matrix.*`, `*.__BASE_DOMAIN__` via
  ClusterIssuer/letsencrypt).
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

## Image policies

Markers reference policy NAMES (`apps:element-web:tag`,
`apps:tuwunel:tag`), not paths. mautrix-discord (v0.7.7) stays pinned,
no policy: upstream is `dock.mau.dev` (manual bumps, see
`base/mautrix-discord.yaml`).

## Per-workload docs

`projects/apprise-go-api/README.md`, `base/NOTIFICATIONS.md`
(Flux→apprise→Matrix wiring + tag/matrix contract).

## Updates

Version sources: image tags in `base/tuwunel.yaml` (auto via
`apps:tuwunel:tag`), `base/element-web.yaml` (auto via
`apps:element-web:tag`), and `base/mautrix-discord.yaml` (bridge
v0.7.7, PINNED — no policy, upstream is `dock.mau.dev`). ImagePolicy PRs land
for tuwunel/element-web; the bridge pin moves by hand after reading its release
notes (config shape is authored from `example-config.yaml` — re-diff on
bumps). tuwunel is a singleton on a single RWO PVC (RocksDB single-writer,
`Recreate`) and `server_name` is IMMUTABLE — export media + snapshot the
`mautrix-discord-db` CNPG cluster before major bumps.
Changelogs: tuwunel https://github.com/matrix-construct/tuwunel/releases ·
element https://github.com/element-hq/element-web/releases · bridge
https://github.com/mautrix/discord/releases.

## Environments

| Env | Hosts | Notable patches |
| --- | --- | --- |
| `dev` | `tuwunel.matrix.home-ops-dev.yansyah.my.id`, `element.matrix.home-ops-dev.yansyah.my.id`, Zitadel `admin.zitadel.home-ops-dev.yansyah.my.id` | vault refs, wildcard cert, hostnames, SERVER_NAME + well-known + issuer + callback, bridge HS link + HS_DOMAIN/admin MXID, element config.json, proxy vars, HPA bounds, DB instances 1 |
| `prd` | `tuwunel.matrix.home-ops.yansyah.my.id`, `element.matrix.home-ops.yansyah.my.id`, Zitadel `admin.zitadel.home-ops.yansyah.my.id` | same shape, DB instances 3, HPA floors 2 |

## Verification

```bash
kustomize build flux/apps/components/matrix/{dev,prd} --load-restrictor=LoadRestrictionsNone | kubeconform -strict -ignore-missing-schemas
./flux/scripts/validate.sh -d flux/apps
```
