# matrix

Single-tenant Matrix stack: one namespace (`matrix`), one OCI artifact
(`apps/matrix`), one Fleet Kustomization. Consolidates the four former
tenants `apprise-go-api`, `tuwunel`, `mautrix-discord`, `element-web` —
they are highly integrated (bridge dials tuwunel, Element points at
tuwunel, Providers post to apprise), never used outside the stack, and
fail together, so one tenant fits the tenant==namespace==artifact
invariant (fleet templates, cosign subject, push path all assume it).

## Workloads

| Workload | Role | Image | Scaling |
|---|---|---|---|
| `apprise-go-api` | Internal ClusterIP webhook sink (`projects/apprise-go-api`): the single target for notification-controller Alert/Provider posts, forwarding to Matrix rooms | `ghcr.io/lazygeniusman/home-ops/projects/apprise-go-api` (`$imagepolicy` → `apps:apprise-go-api:tag`) | HPA 1–2 dev / 2–4 prd, VPA |
| `tuwunel` | Matrix homeserver (`server_name == tuwunel.matrix.<env>`, Zitadel SSO, federation off, RocksDB on S3-backed media) | `ghcr.io/matrix-construct/tuwunel:v1.9.2` (pinned, no policy yet) | Singleton (no HPA), VPA Auto |
| `mautrix-discord` | Discord puppeting bridge (`@discordbot:<server>`) + colocated `mautrix-discord-db` CNPG Cluster | `dock.mau.dev/mautrix/discord:v0.7.7` (pinned, no policy) | Both singleton (bridge 1, DB 1 dev / 3 prd), VPA Initial |
| `element-web` | Public stateless SPA speaking to tuwunel (Gateway + NetBird) | `vectorim/element-web` (`$imagepolicy` → `apps:element-web:tag`) | HPA 1–2 dev / 2–4 prd, VPA Off |

## Layout (environment-direct, apps area)

`base/` holds every manifest; env overlays `{dev,prd}/` patch hostnames,
vault refs, identity values, and replica bounds via
`resources: [../base]` + RFC-6902 patches (each patch carries a
`---- <workload> (carried over from components/<old>/…) ----` section
header so its origin stays auditable).

## Merges (from the 4 former components)

- `wildcard-certificate.yaml`: ONE copy — the tuwunel and element-web
  Certificates were spec-identical (`*.__BASE_DOMAIN__` via
  ClusterIssuer/letsencrypt), covering both nested hostnames
  (`tuwunel.matrix.*` + `element.matrix.*`). The 4 identical overlay
  cert/token patches (tuwunel's + element-web's per env) dedupe to one
  set (the element copies are recorded as dropped-duplicate comments).
- `tuwunel-storage.yaml` + `mautrix-storage.yaml`: kept SEPARATE —
  different buckets/purposes (tuwunel app media vs CNPG WAL+base
  backups); merging would mix media with DB backups. (Renamed from
  `bucketclaims.yaml` to avoid the filename collision.)
- `tuwunel-cosi-keys.yaml` + `mautrix-cosi-keys.yaml`: kept SEPARATE —
  distinct SecretStores (plus tuwunel's file also holds the `tuwunel-s3`
  ExternalSecret; mautrix's `cnpg-s3-credentials` ES lives in
  `mautrix-discord-db.yaml`). All `remoteNamespace:` values are now
  `matrix`. (Renamed from `cosi-keys.yaml` to avoid the collision.)
- `matrix-rbac.yaml`: ONE shared `eso-k8s-reader`
  ServiceAccount/Role/Binding — the tuwunel and mautrix-discord copies
  were identical apart from labels, serving every in-namespace k8s store
  (`tuwunel-k8s`, `tuwunel-cosi`, `mautrix-discord-cosi`).
- `cloudflare-api-token`: ONE ExternalSecret — the tuwunel and
  element-web mirrors were identical, so the tuwunel copy (inside
  `tuwunel-secrets.yaml`) survives and `element-secrets.yaml` is NOT
  carried over.
- HTTPRoutes keep distinct names (`tuwunel-redirect` + `tuwunel-tls` +
  `element`) — no rename needed. `mautrix-discord-db.yaml` (CNPG),
  `element-proxy.yaml` (NetBird Terraform), `notifications.yaml` +
  apprise secrets ship as-is (Provider addresses now
  `http://apprise-go-api.matrix.svc:80/…` FQDN — the bare short name
  only resolved inside the old per-tenant namespace layout).

## Image policies

Markers reference policy NAMES (`apps:apprise-go-api:tag`,
`apps:element-web:tag`), not paths — `flux/apps/update-policies/*.yaml`
needs no edits and both markers survive in the moved files. tuwunel
(v1.9.2) + mautrix-discord (v0.7.7) stay pinned, no policy (as before).
Follow-up: optionally add `update-policies/tuwunel.yaml` + `$imagepolicy`
marker (see the note in `base/tuwunel.yaml`).

## Per-workload docs

Full design notes live on: `projects/apprise-go-api/README.md`,
`base/NOTIFICATIONS.md` (Flux→apprise→Matrix wiring + tag/matrix
contract), and the per-area sections above. The four old component
READMEs were removed with their directories — consult git history
(`apprise-go-api`, `tuwunel`, `mautrix-discord`, `element-web`) for the
pre-consolidation narratives.

## Environments

| Env | Hosts | Notable patches |
| --- | --- | --- |
| `dev` | `tuwunel.matrix.homelab-dev.yansyah.my.id`, `element.matrix.homelab-dev.yansyah.my.id`, Zitadel `admin.zitadel.homelab-dev.yansyah.my.id` | vault refs, wildcard cert, hostnames, SERVER_NAME + well-known + issuer + callback, bridge HS link (`http://tuwunel.matrix.svc:8008`) + dev HS_DOMAIN/admin MXID, element config.json, proxy vars, HPA bounds, DB instances 1 |
| `prd` | `tuwunel.matrix.home-ops.yansyah.my.id`, `element.matrix.home-ops.yansyah.my.id`, Zitadel `admin.zitadel.home-ops.yansyah.my.id` | same shape, DB instances 3, HPA floors 2 |

## Verification

```bash
kustomize build flux/apps/components/matrix/{dev,prd} --load-restrictor=LoadRestrictionsNone | kubeconform -strict -ignore-missing-schemas
./flux/scripts/validate.sh -d flux/apps
```
