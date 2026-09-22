# Flux -> apprise-go-api -> Matrix wiring (Goal 3)

Greenfield Flux notification wiring: every Flux-native event source emits
via the infra apprise-go-api sink
(`http://apprise-go-api.apprise-go-api.svc:80/notify` — workload lives in
the infra `apprise-go-api` tenant, NOT this one) into per-purpose Matrix
rooms provisioned by the reusable rooms module. notification-controller
is enabled on both clusters; before this change zero Provider/Alert
existed.

Scope: `base/notifications.yaml` (4 generic Providers + 4 Alerts) +
`base/apprise-go-api-secrets.yaml` (STATELESS_URLS fallback) + room example
CRs + this doc. Tenant/workflow onboarding (`tenants/apps.yaml`,
`flux-apps-push.yaml`) is a separate task. NO live alert firing here.

## Room -> source -> tag matrix

| Room (`#alias`) | Alert(s) | Event sources (severity) | Provider (`?tag=` / `?type=`) | Fallback URL leg (`?tag=` server tag) |
|---|---|---|---|---|
| `#flux-notifications` | `flux-info` | GitRepository, OCIRepository, Kustomization, HelmRelease (`info`; errors included) minus `^Reconciliation finished.*no changes$` | `apprise-flux` (`flux` / `info`) | `.../%23flux-notifications?tag=flux&...` |
| `#flux-notifications` | `flux-errors` | same four kinds (`error` only) | `apprise-flux-errors` (`flux` / `failure`) | same `tag=flux` leg (distinct `type=` rendering) |
| `#tofu-runs` | `tofu-runs` | Kustomization, HelmRelease (`info`) + `inclusionList: ["(?i)terraform\|tofu"]` | `apprise-tofu` (`tofu` / `info`) | `.../%23tofu-runs?tag=tofu&...` |
| `#team` (opt-in template) | `team-optin` (SUSPENDED; copy + set namespace + unsuspend) | Kustomization (`info`, team namespace) minus no-change noise | `apprise-team` (`team` / `info`) | `.../%23team?tag=team&...` |

Notes:

- `flux-errors` and `flux-info` share `#flux-notifications`: failures
  arrive twice (info stream + `type=failure` highlight) by design — the ops
  overview never misses a failure even if the info stream is muted.
- `tofu-runs` overlaps `flux-info` the same way: tofu events post to BOTH
  rooms (quiet per-run log vs ops overview). `Terraform` is NOT a valid
  Alert `eventSources` kind (schema enum has no Terraform entry), so the
  tofu Alert watches Kustomization + HelmRelease and filters on message
  text via `inclusionList`.
- `eventSeverity: info` INCLUDES errors (info = no filtering); `error`
  filters to errors only. `exclusionList`/`inclusionList` are Golang RE2
  regexes matched against the event message.
- Per-team onboarding: copy the `team-optin` Alert, set
  `eventSources[].namespace` to the team tenant namespace, and either reuse
  the literal `#team` alias (single shared opt-in room) or add a
  `?tag=<team>` fallback leg for the team's own alias + a matching
  Provider. Unsuspend only after the room exists.
- Matrix-first, fallback-second: the `APPRISE_STATELESS_URLS` fallback IS
  the Matrix leg (per-room URLs above). `APPRISE_WEBHOOK_URL` (outbound
  result hook: `{"source","status","output"}` POST after every notify) is
  UNSET — no second sink exists yet; wire one only when a fallback
  consumer lands.

## Tag / priority scheme

- Request `?tag=` (on every Provider address) filters against the fallback
  URLs' own `?tag=` server tags (engine: `tagMatchesURL`; nil filter =
  match-all). A token matches a server tag name-only (case-insensitive,
  priority/retry decorations ignored) or exactly on `name+priority`; the
  `all` token matches everything. A mismatch selects ZERO targets ->
  HTTP 204 (SILENT — the selection is lost, no fallback fires).
- Rule: every Provider `tag=` MUST equal a tag baked into a
  `STATELESS_URLS` entry (`flux` | `tofu` | `team`). Priority prefixes
  (`N:tag`) are parsed but unused — flat purpose tags are enough for
  three rooms; add priorities only if one room needs severity fan-out.
- `overflow=split` on every fallback leg: over-limit bodies continue in
  additional messages (never truncated silently). `?mode=` is ABSENT on
  purpose — webhook modes (`matrix`/`slack`/`hookshot`) apply only to
  webhook-mode rooms, and these are Client-API rooms.

## Apprise payload mapping (generic webhook JSON -> notify)

Flux generic Provider POSTs a JSON `Event`
(`involvedObject{apiVersion,kind,name,namespace,uid}`,
`metadata{...revision...}`, `severity`, `reason`, `message`,
`reportingController`, `reportingInstance`, `timestamp`) with a
`Gotk-Component` header. The Provider address maps it onto apprise fields:

- `?:message=body` — flat rename: Flux `message` (long human text) ->
  apprise `body`. Source wins; both sides expected -> swap (never the
  case here — `message` is not an apprise field).
- `?:reason=title` — flat rename: Flux `reason` (short machine string,
  e.g. `ReconciliationSucceeded`) -> apprise `title`.
- `?format=text` — Flux messages are plain text (Flux docs example);
  markdown/html would attach a second fallback body for no benefit.
  `?title=`/`?type=` query fallbacks apply only when the body has no
  value — the remap above always sets both, so the pinned `?type=` wins.
- `?type=info|failure` — pinned per Provider. Flux `severity`
  (`info|error`) is NEVER remapped: `error` is not a valid apprise `type`
  (`info|success|warning|failure` only — `validNotifyType`) and would 400
  (`Payload lacks minimum requirements`). The Alert `eventSeverity`
  selects the pipeline instead (info leg vs error leg).
- Remap failure (missing `message`/`reason`) -> HTTP 400 `Payload field
  mapping failed` (notification-controller retries, then drops; surfaced
  in its logs, NOT Matrix).
- `events/` attachment path: Flux events carry no attachments; `attach`
  stays unset. `APPRISE_ATTACH_*` SSRF posture unchanged.
- Outbound `APPRISE_WEBHOOK_URL`: unset (no second sink yet).

## E2EE-vs-plaintext decision: PLAINTEXT for notifier rooms

- All three purpose rooms are `encryption_enabled: false` (room examples:
  `encryption_enabled: false`, `events_default: 50`), and every fallback
  URL carries `e2ee=false`.
- Why: the apprise-go Matrix target defaults to `e2ee=true`, but E2EE
  needs a persistent Olm account + device identity (`matrix_e2ee.go`:
  "have to outlive the process ... register a new device on every
  notification"). apprise-go-api is stateless by design
  (`APPRISE_STATELESS_STORAGE=no`, no volumes) — every Pod restart would
  mint a new unverified device and previously-sent content stays
  unreadable. Plaintext is the only operable mode for a bot sink.
- Limits (apprise docs, Matrix service page): plaintext bodies 60k chars /
  HTML-Markdown 29k (vs E2EE 40k / 19k). `overflow=split` covers the tail.
  Matrix caps the whole event at 65,536 bytes regardless.
- E2EE stays for human/opt-in rooms: any room humans converse in SHOULD
  use the module default (`encryption_enabled: true`, irreversible —
  decide at creation, never flip). Notifier rooms are bot-write/human-read
  only, so plaintext exposure is limited to Flux event text (no secrets —
  event messages must never carry credentials; audit `message` content if
  a controller starts echoing secret material).
- `?mode=` stays off: webhook modes are for webhook-mode rooms, not
  Client-API plaintext rooms.
- Revisit only if: apprise-go-api gains a persistent Olm store (new
  volume + device-identity lifecycle) — then re-evaluate per room.

## Room locks (reusable module)

Bot pinned at power 100 by the module (self-lockout safe); per-room:
on-call/team lead at 50 (invite/kick/redact/state), `users_default: 0`,
`events_default: 50` (notification-only — normal members CANNOT post),
`state_default: 50`, `join_rule: invite`, `visibility: private`,
`history_visibility: shared`, `preset: private_chat`. Room IDs/aliases
reach apprise as `matrixs://` URLs (token-auth form
`matrixs://{token}@{host}/%23{alias}?tag=...` — `%23`, never literal `#`;
room-ID `!...` targets need no encoding). No secrets in Git: bot token +
homeserver host flow from Proton Pass through ESO (`varsFrom` for rooms,
`apprise-stateless-urls` for the fallback).

## Deferred (explicitly NOT this change)

- Zitadel SMTP stays on the future mail relay — NOT apprise (email needs
  SMTP credentials per recipient domain; apprise `email://` URLs would
  spray vault-held mail passwords into request bodies).
- Prometheus `AlertmanagerConfig` / `PrometheusRule` reserved until the
  monitoring stack lands (Flux Alert CRs here cover Flux-native sources
  only).
- Proton Pass re-check: the `notifier-bot-token` + `homeserver-host`
  fields compose into an authenticated `matrixs://` URL (hidden-webhook
  class secret). Re-check vault item visibility/sharing before pasting
  anywhere; room aliases are public, tokens never are.
- `generic-hmac` parity (`SECRET_KEY(_FILE)` + `X-Signature` verification)
  only if apprise gains signature verification — today the header would
  be silently ignored.
- Workload placement: the apprise-go-api Deployment/Service/HPA/VPA now
  live in the infra `apprise-go-api` tenant
  (`flux/infra/components/apprise-go-api`, credential-free); this tenant
  keeps only the consumer-owned `apprise-stateless-urls` fallback +
  Provider/Alert wiring. Per-request `urls` injection (dropping the
  STATELESS_URLS fallback entirely) is follow-up work — the sink already
  supports it (missing `urls` -> 204, no crash).
- Fleet tenants/workflows onboarding (`tenants/apps.yaml` + image-update
  policies + `flux-apps-push.yaml` matrix): separate task.

## End-to-end test plan (plan-only — no cluster access here)

1. Preconditions (kubectl, plan-only): `kubectl get providers,alerts -A`
   shows `apprise-*` Ready=True; `kubectl get externalsecret
   apprise-stateless-urls -n <tenant>` Synced; `flux-notifications-outputs`
   / `tofu-runs-outputs` Secrets exist (room IDs minted).
2. Trigger: `flux reconcile kustomization <tenant>-apps --with-source`
   (info path) and a forced failure (bad image tag, then revert) for the
   error leg. Suspend first if noise matters:
   `flux suspend alert flux-info` / `flux resume alert flux-info`.
3. Expect: apprise `POST /notify` returns 200
   (`{"error":null,"details":[...]}`); both `#flux-notifications`
   (info + failure highlight) and `#tofu-runs` (tofu runs only) receive
   messages; apprise logs show per-target delivery, no 204/400/424.
4. Negative checks: unknown `?tag=` -> 204 (selection lost — confirms the
   tag-equality rule); `message`-less probe POST -> 400 mapping failure;
   missing `apprise-stateless-urls` Secret -> Pod still runs (optional
   ref), requests without `urls` 204.
5. Readiness: notification-controller marks Provider+Alert Ready
   (`flux get alerts --all-namespaces`); `Alert.Status.ObservedGeneration`
   advances after reconcile events.
