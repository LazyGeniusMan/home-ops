# matrix

Single-tenant Matrix stack: one namespace (`matrix`), one OCI artifact
(`apps/matrix`), one Fleet Kustomization. Holds the `tuwunel` homeserver,
the `mautrix-discord` bridge (+ colocated Postgres), the `element-web` SPA,
and consumer-only apprise wiring (the `apprise-go-api` workload lives in the
infra `apprise-go-api` tenant as shared credential-free platform plumbing).
They are highly integrated (bridge dials tuwunel, Element points at
tuwunel, Providers post cross-namespace to the infra sink), never used
outside the stack, and fail together, so one tenant fits the
tenant==namespace==artifact invariant (fleet templates, cosign subject,
push path all assume it).

## Workloads

| Workload | Role | Image | Scaling |
|---|---|---|---|
| `apprise-go-api` (consumer only) | NO workload ships here — the sink lives in the infra `apprise-go-api` tenant (namespace `apprise-go-api`, `flux/infra/components/apprise-go-api`). This tenant keeps the consumer-owned `apprise-stateless-urls` fallback ExternalSecret + Provider/Alert wiring, posting to `http://apprise-go-api.apprise-go-api.svc:80/notify` | n/a (policy `infra:apprise-go-api:tag` owns the image) | n/a (infra HPA 1–2 dev / 2–4 prd, VPA) |
| `tuwunel` | Matrix homeserver (`server_name == tuwunel.matrix.<env>`, Zitadel SSO, federation off, RocksDB on S3-backed media) | `ghcr.io/matrix-construct/tuwunel` (`$imagepolicy` → `apps:tuwunel:tag`) | Singleton (no HPA), VPA Auto |
| `mautrix-discord` | Discord puppeting bridge (`@discordbot:<server>`) + colocated `mautrix-discord-db` CNPG Cluster | `dock.mau.dev/mautrix/discord:v0.7.7` (pinned, no policy) | Both singleton (bridge 1, DB 1 dev / 3 prd), VPA Initial |
| `element-web` | Public stateless SPA speaking to tuwunel (Gateway + NetBird) | `vectorim/element-web` (`$imagepolicy` → `apps:element-web:tag`) | HPA 1–2 dev / 2–4 prd, VPA Off |

## Layout (environment-direct, apps area)

`base/` holds every manifest; env overlays `{dev,prd}/` patch hostnames,
vault refs, identity values, and replica bounds via
`resources: [../base]` + RFC-6902 patches, grouped under per-workload
`---- <workload> ----` section headers.

## Shared files (one copy serves the whole tenant)

- `wildcard-certificate.yaml`: ONE copy covering both nested hostnames
  (`tuwunel.matrix.*` + `element.matrix.*`, `*.__BASE_DOMAIN__` via
  ClusterIssuer/letsencrypt).
- `tuwunel-storage.yaml` + `mautrix-storage.yaml`: SEPARATE —
  different buckets/purposes (tuwunel app media vs CNPG WAL+base
  backups); mixing them would put media and DB backups in one bucket.
- `tuwunel-cosi-keys.yaml` + `mautrix-cosi-keys.yaml`: SEPARATE —
  distinct SecretStores (plus tuwunel's file also holds the `tuwunel-s3`
  ExternalSecret; mautrix's `cnpg-s3-credentials` ES lives in
  `mautrix-discord-db.yaml`). All `remoteNamespace:` values are `matrix`.
- `matrix-rbac.yaml`: ONE shared `eso-k8s-reader`
  ServiceAccount/Role/Binding serving every in-namespace k8s store
  (`tuwunel-k8s`, `tuwunel-cosi`, `mautrix-discord-cosi`).
- `cloudflare-api-token`: ONE ExternalSecret (inside
  `tuwunel-secrets.yaml`).
- HTTPRoutes keep distinct names (`tuwunel-redirect` + `tuwunel-tls` +
  `element`). `mautrix-discord-db.yaml` (CNPG),
  `element-proxy.yaml` (NetBird Terraform), `notifications.yaml` +
  apprise secrets ship as consumer-only wiring (Provider addresses are the
  infra Service DNS `http://apprise-go-api.apprise-go-api.svc:80/…` —
  cross-namespace, since the workload lives in the infra tenant).

## Image policies

Markers reference policy NAMES (`apps:element-web:tag`,
`apps:tuwunel:tag`), not paths — their `flux/apps/update-policies/*.yaml`
policies track upstream (the apprise-go-api marker lives on the infra
workload as `infra:apprise-go-api:tag`, owned by
`flux/infra/update-policies/apprise-go-api.yaml`). mautrix-discord
(v0.7.7) stays pinned, no policy:
upstream is `dock.mau.dev` (manual bumps per the note in
`base/mautrix-discord.yaml`).

## Per-workload docs

Full design notes live on: `projects/apprise-go-api/README.md`,
`base/NOTIFICATIONS.md` (Flux→apprise→Matrix wiring + tag/matrix
contract), and the per-area sections above.

## Upgrade runbook

- Version source: image tags in `base/tuwunel.yaml` (tuwunel, auto via
  `apps:tuwunel:tag`), `base/element-web.yaml` (element-web, auto via
  `apps:element-web:tag`), and `base/mautrix-discord.yaml` (bridge
  v0.7.7, PINNED — no policy, upstream is `dock.mau.dev`).
- Changelog (tuwunel): https://github.com/matrix-construct/tuwunel/releases.
  Changelog (element): https://github.com/element-hq/element-web/releases.
  Changelog (bridge): https://github.com/mautrix/discord/releases.
- Bump: let the tuwunel/element-web ImagePolicy PRs land
  (`update-policies/tuwunel.yaml`, `update-policies/element-web.yaml`);
  move the bridge pin by hand after reading its release notes (config
  shape is authored from `example-config.yaml` — re-diff on bumps).
- Migrate: tuwunel is a singleton on a single RWO PVC (RocksDB
  single-writer, `Recreate`) and `server_name` is IMMUTABLE — export
  media + snapshot the `mautrix-discord-db` CNPG cluster BEFORE tuwunel
  major bumps. Verify: send a message end-to-end (Element → tuwunel →
  bridged Discord room) in each env.

## Environments

| Env | Hosts | Notable patches |
| --- | --- | --- |
| `dev` | `tuwunel.matrix.home-ops-dev.yansyah.my.id`, `element.matrix.home-ops-dev.yansyah.my.id`, Zitadel `admin.zitadel.home-ops-dev.yansyah.my.id` | vault refs, wildcard cert, hostnames, SERVER_NAME + well-known + issuer + callback, bridge HS link (`http://tuwunel.matrix.svc:8008`) + dev HS_DOMAIN/admin MXID, element config.json, proxy vars, HPA bounds, DB instances 1 |
| `prd` | `tuwunel.matrix.home-ops.yansyah.my.id`, `element.matrix.home-ops.yansyah.my.id`, Zitadel `admin.zitadel.home-ops.yansyah.my.id` | same shape, DB instances 3, HPA floors 2 |

## Verification

```bash
kustomize build flux/apps/components/matrix/{dev,prd} --load-restrictor=LoadRestrictionsNone | kubeconform -strict -ignore-missing-schemas
./flux/scripts/validate.sh -d flux/apps
```
