# Flux -> apprise-go-api -> Matrix wiring

Every Flux-native event source emits via the infra sink (`http://apprise-go-api.apprise-go-api.svc:80/notify`) into per-purpose Matrix rooms. notification-controller is enabled on both clusters.

Scope: `base/notifications.yaml` (4 Providers + 4 Alerts; Coder posts directly) + `base/apprise-go-api-secrets.yaml` (STATELESS_URLS fallback) + `base/matrix-bot-bootstrap.yaml` (bot Job + kept Secret) + `base/rooms.yaml` CRs + `terraform/examples/` skeletons.

## Fallback-leg ownership

| Leg / room | Owner | Credential path |
|---|---|---|
| `tag=flux` / `#flux-notifications` | matrix tenant | kept Secret -> `tuwunel-k8s` ES -> `apprise-stateless-urls` |
| `tag=tofu` / `#tofu-runs` | matrix tenant | (same ES, same Secret) |
| `tag=team` / `#team` (opt-in) | matrix tenant | (same ES, same Secret) |
| (no `tag=coder`) / `#coder-notifications` | coder app | kept Secret -> `coder-matrix` handoff -> ES `matrix-notify` -> per-request `urls` |

## Room -> source -> tag matrix

| Room (`#alias`) | Alert(s) | Event sources (severity) | Provider (`?tag=` / `?type=`) | Fallback leg |
|---|---|---|---|---|
| `#flux-notifications` | `flux-info` | GitRepository, OCIRepository, Kustomization, HelmRelease (`info`, incl. errors) minus `^Reconciliation finished.*no changes$` | `apprise-flux` (`flux` / `info`) | `?tag=flux` |
| `#flux-notifications` | `flux-errors` | same four kinds (`error` only) | `apprise-flux-errors` (`flux` / `failure`) | same `tag=flux` leg |
| `#tofu-runs` | `tofu-runs` | Kustomization, HelmRelease (`info`) + `inclusionList: ["(?i)terraform\|tofu"]` | `apprise-tofu` (`tofu` / `info`) | `?tag=tofu` |
| `#team` (opt-in) | `team-optin` (SUSPENDED) | Kustomization (`info`, team namespace) minus no-change noise | `apprise-team` (`team` / `info`) | `?tag=team` |
| `#coder-notifications` | (none — Coder posts directly) | Coder webhook (every event fans into this ONE room) | (none — `?type=info&format=text` + per-request `urls`) | (none) |

Notes: `flux-errors` + `flux-info` share `#flux-notifications` (failures arrive twice); `tofu-runs` overlaps `flux-info` the same way. `Terraform` is not a valid `eventSources` kind — the tofu Alert watches Kustomization + HelmRelease + `inclusionList`. `info` includes errors; `error` filters to errors only. Unsuspend `team-optin` only after the room exists. `APPRISE_STATELESS_URLS` is the Matrix leg; `APPRISE_WEBHOOK_URL` unset.

## Tag scheme + payload mapping

Request `?tag=` must equal a `STATELESS_URLS` tag (`flux` | `tofu` | `team`); a mismatch selects zero targets → HTTP 204 (silent). `overflow=split` on every leg. Provider mapping (Flux JSON `Event` → apprise): `?:message=body` + `?:reason=title` renames, `?format=text`, pinned `?type=info|failure` (severity selects the pipeline, never remapped). Failure → HTTP 400 (retries, then drops). No attachments.

## Coder flow

Coder webhook (`CODER_NOTIFICATIONS_METHOD=webhook`, unsigned POST) → sink `/notify` with per-request `urls` (body) → `#coder-notifications`. Sink contract: `projects/apprise-go-api/internal/server/notify.go` (`urls` is body-only). One global endpoint, one room.

## Bot bootstrap chain

Vault `matrix/tuwunel-registration-secret` (one first-seed per env) → ES + Secret → tuwunel mount + Job env → Job (nonce → HMAC-SHA1 register → login → token) → kept Secret: `homeserver_url`/`access_token`/`user_id` (rooms `varsFrom`) + `notifier-token`/`homeserver-host` (apprise ES). One bot per env (`@apprise-dev` dev / `@apprise` prd), shared by all 3 rooms. Rerun = delete Job + reconcile.

## Room locks

Notifier rooms are plaintext (`encryption_enabled: false` — the stateless bot cannot hold an E2EE device identity; human rooms should be encrypted, irreversible, decide at creation). Bot pinned at power 100 (self-lockout safe); per-room: lead at 50, `users_default: 0`, `events_default: 50`, `join_rule: invite`, `visibility: private`, `history_visibility: shared`. Apprise URLs use `%23`, never literal `#`.
