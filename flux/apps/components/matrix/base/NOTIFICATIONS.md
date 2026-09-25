# Flux -> apprise-go-api -> Matrix wiring

Every Flux-native event source emits via the infra apprise-go-api sink
(`http://apprise-go-api.apprise-go-api.svc:80/notify` — workload lives in the
infra tenant, NOT this one) into per-purpose Matrix rooms from the reusable rooms
module. notification-controller is enabled on both clusters.

Scope: `base/notifications.yaml` (4 generic Providers + 4 Alerts; Coder posts
directly, no `apprise-coder` Provider) + `base/apprise-go-api-secrets.yaml`
(STATELESS_URLS fallback: flux/tofu/team legs) + `base/matrix-bot-bootstrap.yaml`
(bot Job + kept Secret) + live `base/rooms.yaml` CRs + `terraform/examples/` skeletons.

## Fallback-leg ownership (decoupled — read first)

| Leg / room | Owner | Credential path |
|---|---|---|
| `tag=flux` / `#flux-notifications` | matrix tenant | kept Secret -> `tuwunel-k8s` ES -> `apprise-stateless-urls` |
| `tag=tofu` / `#tofu-runs` | matrix tenant | (same ES, same Secret) |
| `tag=team` / `#team` (opt-in) | matrix tenant | (same ES, same Secret) |
| (no `tag=coder`) / `#coder-notifications` | coder app | kept Secret -> coder-owned handoff (`coder-matrix` store + `coder-matrix-handoff-reader` RBAC, both coder-owned) -> coder ES `matrix-notify` -> per-request `urls` |

## Room -> source -> tag matrix

Single contract table (see `base/notifications.yaml`):

| Room (`#alias`) | Alert(s) | Event sources (severity) | Provider (`?tag=` / `?type=`) | Fallback leg |
|---|---|---|---|---|
| `#flux-notifications` | `flux-info` | GitRepository, OCIRepository, Kustomization, HelmRelease (`info`, incl. errors) minus `^Reconciliation finished.*no changes$` | `apprise-flux` (`flux` / `info`) | `?tag=flux` |
| `#flux-notifications` | `flux-errors` | same four kinds (`error` only) | `apprise-flux-errors` (`flux` / `failure`) | same `tag=flux` leg |
| `#tofu-runs` | `tofu-runs` | Kustomization, HelmRelease (`info`) + `inclusionList: ["(?i)terraform\|tofu"]` | `apprise-tofu` (`tofu` / `info`) | `?tag=tofu` |
| `#team` (opt-in) | `team-optin` (SUSPENDED) | Kustomization (`info`, team namespace) minus no-change noise | `apprise-team` (`team` / `info`) | `?tag=team` |
| `#coder-notifications` | (none — Coder posts directly, no Alert/Provider) | Coder webhook (every event fans into this ONE room) | (none — endpoint `?type=info&format=text` + per-request `urls`, bare mapping) | (none) |

Notes: `flux-errors` + `flux-info` share `#flux-notifications` (failures arrive
twice); `tofu-runs` overlaps `flux-info` the same way. `Terraform` is NOT a
valid `eventSources` kind — the tofu Alert watches Kustomization + HelmRelease
+ `inclusionList`. `info` INCLUDES errors; `error` filters to errors only.
Per-team onboarding: copy `team-optin`, set the team namespace, reuse `#team`
or add a `?tag=<team>` leg + Provider. Unsuspend only after the room exists.
`APPRISE_STATELESS_URLS` IS the Matrix leg. `APPRISE_WEBHOOK_URL` UNSET.

## Tag scheme + payload mapping

Request `?tag=` filters against the fallback URLs' own `?tag=` server tags; a
mismatch selects ZERO targets -> HTTP 204 (SILENT). Every Provider `tag=` MUST
equal a `STATELESS_URLS` tag (`flux` | `tofu` | `team`; `coder` is NOT a
fallback tag). `overflow=split` on every leg. Provider address mapping (Flux
JSON `Event` -> apprise): `?:message=body` + `?:reason=title` renames,
`?format=text`, pinned `?type=info|failure` (Flux `severity` never remapped —
the Alert `eventSeverity` selects the pipeline). Remap failure -> HTTP 400
(retries, then drops). No attachments; `attach` unset.

## Coder decoupled flow (webhook JSON -> notify, per-request `urls`)

Coder's webhook method (`CODER_NOTIFICATIONS_METHOD=webhook`) sends an UNSIGNED
HTTP POST to `CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT` — NOT a Provider address
(Alerts cannot watch Coder; the sink accepts unsigned POSTs, so no auth headers
are needed). Data flow: kept Secret -> coder-owned handoff (`coder-matrix`
store + RBAC) -> ES `matrix-notify` -> HelmRelease env -> sink POST /notify
per-request `urls` (body) -> #coder-notifications. Coder's fixed payload
already carries top-level `title` + `body`, so the sink binds them with zero
remapping; `?format=text`, `?type=info` pinned. NO `?tag=coder` — body `urls`
select the target directly. Sink contract:
`projects/apprise-go-api/internal/server/notify.go` (`urls` is body-only, no
`?urls=` query fallback). Single-endpoint fan-in: Coder exposes ONE global
webhook endpoint, so every event lands in this single room.

## Bot bootstrap chain

Vault `matrix/tuwunel-registration-secret` (ONE first-seed per env) -> ES +
Secret -> tuwunel mount + Job env -> Job (nonce -> HMAC-SHA1 register -> login
-> token) -> KEPT Secret (no ownerRef): `homeserver_url`/`access_token`/`user_id`
(rooms varsFrom) + `notifier-token`/`homeserver-host` (apprise ES) + url/id
aliases (same-namespace reads). Single seeded vault path (see VAULT-SEEDS.md).
ONE bot per env (`@apprise-dev` dev / `@apprise` prd) shared by all 3 rooms.
Rerun = delete Job + reconcile.

## Room locks (reusable module)

Notifier rooms are plaintext (`encryption_enabled: false`, `events_default: 50`,
`e2ee=false` — the stateless bot sink cannot hold an E2EE device identity;
human rooms SHOULD be encrypted, irreversible, decide at creation). Bot pinned
at power 100 by the module (self-lockout safe); per-room: on-call/team lead at
50, `users_default: 0`, `events_default: 50` (notification-only), `state_default:
50`, `join_rule: invite`, `visibility: private`, `history_visibility: shared`,
`preset: private_chat`. Apprise URLs use `%23`, never literal `#`. No secrets in
Git: bot token + host flow from the bootstrap kept Secret (rooms via `varsFrom`,
fallback via the in-cluster `tuwunel-k8s` ES; coder via the coder-owned handoff
into `matrix-notify`). The apprise-go-api workload lives in the infra tenant
(`flux/infra/components/apprise-go-api`, credential-free); this tenant keeps
only the consumer-owned fallback + Provider/Alert wiring.
