# Flux -> apprise-go-api -> Matrix wiring (Goal 3)

Greenfield Flux notification wiring: every Flux-native event source emits
via the infra apprise-go-api sink
(`http://apprise-go-api.apprise-go-api.svc:80/notify` — workload lives in
the infra `apprise-go-api` tenant, NOT this one) into per-purpose Matrix
rooms provisioned by the reusable rooms module. notification-controller
is enabled on both clusters; before this change zero Provider/Alert
existed.

Scope: `base/notifications.yaml` (4 generic Providers + 4 Alerts — the
`apprise-coder` Provider is REMOVED, Coder posts directly) +
`base/apprise-go-api-secrets.yaml` (STATELESS_URLS fallback: flux/tofu/team
legs only) + `base/matrix-bot-bootstrap.yaml` (bot bootstrap Job + kept
Secret) + room example CRs + live `base/rooms.yaml` CRs + this doc.
Tenant/workflow onboarding (`tenants/apps.yaml`, `flux-apps-push.yaml`) is
a separate task. NO live alert firing here.

## Fallback-leg ownership (decoupled — read first)

| Leg / room | Owner | Credential path | Why here |
|---|---|---|---|
| `tag=flux` / `#flux-notifications` | matrix tenant | bootstrap kept Secret -> `tuwunel-k8s` ES -> `apprise-stateless-urls` | room is matrix-tenant-owned; centralization legitimate |
| `tag=tofu` / `#tofu-runs` | matrix tenant | (same ES, same Secret) | (same) |
| `tag=team` / `#team` (opt-in) | matrix tenant | (same ES, same Secret) | (same) |
| (none — NO `tag=coder`) / `#coder-notifications` | coder app | kept Secret `matrix-bot-bootstrap-outputs` (`notifier-token`/`homeserver-host`) -> coder-owned handoff (SecretStore `coder-matrix` remoteNamespace `matrix` + Role/Binding `coder-matrix-handoff-reader` in ns `matrix`, owned by coder component) -> coder ES `matrix-notify` -> `CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT` (per-request `urls`); vault `coder/matrix-*` retired | Coder MUST own its credential in its own deployment; the matrix tenant carries NO coder leg (centralized fallback for coder is dead weight AND a coupling violation — the infra sink is credential-free, so an unconsumed leg 204s silently) |

## Room -> source -> tag matrix

| Room (`#alias`) | Alert(s) | Event sources (severity) | Provider (`?tag=` / `?type=`) | Fallback URL leg (`?tag=` server tag) |
|---|---|---|---|---|
| `#flux-notifications` | `flux-info` | GitRepository, OCIRepository, Kustomization, HelmRelease (`info`; errors included) minus `^Reconciliation finished.*no changes$` | `apprise-flux` (`flux` / `info`) | `.../%23flux-notifications?tag=flux&...` |
| `#flux-notifications` | `flux-errors` | same four kinds (`error` only) | `apprise-flux-errors` (`flux` / `failure`) | same `tag=flux` leg (distinct `type=` rendering) |
| `#tofu-runs` | `tofu-runs` | Kustomization, HelmRelease (`info`) + `inclusionList: ["(?i)terraform\|tofu"]` | `apprise-tofu` (`tofu` / `info`) | `.../%23tofu-runs?tag=tofu&...` |
| `#team` (opt-in template) | `team-optin` (SUSPENDED; copy + set namespace + unsuspend) | Kustomization (`info`, team namespace) minus no-change noise | `apprise-team` (`team` / `info`) | `.../%23team?tag=team&...` |
| `#coder-notifications` | (none — Coder posts directly, NOT via Alert; NO Provider ships here) | Coder Deployment webhook (`CODER_NOTIFICATIONS_METHOD=webhook`; every event fans into this ONE room) | (none — endpoint `.../notify/?type=info&format=text` + per-request `urls` in body, bare mapping — no remap) | (none — NO fallback leg; coder-owned per-request `urls` below) |

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
  `STATELESS_URLS` entry (`flux` | `tofu` | `team` — `coder` is NOT a
  fallback tag anymore). Priority prefixes (`N:tag`) are parsed but unused
  — flat purpose tags are enough for three fallback legs + one decoupled
  caller; add priorities only if one room needs severity fan-out.
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

## Coder decoupled flow (webhook JSON -> notify, per-request `urls`)

Coder's webhook delivery method (`CODER_NOTIFICATIONS_METHOD=webhook`)
sends an UNSIGNED HTTP POST to `CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT` —
NOT a Provider address (no `apprise-coder` Provider ships; Alert
eventSources cannot watch Coder, and a Provider referencing the removed
`?tag=coder` leg would 204). The generic apprise-go-api sink accepts
unsigned POSTs, so no auth headers are needed.

Data-flow trace (kept Secret -> coder handoff -> ES -> env -> sink urls,
zero matrix-tenant secret involved; vault coder/matrix-* retired):

```text
kept Secret matrix-bot-bootstrap-outputs (ns matrix;
  keys notifier-token/homeserver-host)
  -> coder-owned handoff (SecretStore coder-matrix remoteNamespace matrix +
     Role/Binding coder-matrix-handoff-reader in ns matrix, owned by the
     coder component)
  -> ES matrix-notify (coder/base/coder-secrets.yaml, target.template:
     apprise-urls + webhook-endpoint)
  -> Secret matrix-notify (keys apprise-urls, webhook-endpoint)
  -> HelmRelease env valueFrom.secretKeyRef
     (CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT <- webhook-endpoint;
      CODER_MATRIX_APPRISE_URLS <- apprise-urls)
  -> sink POST /notify per-request `urls` (body form field)
  -> #coder-notifications (matrixs://{token}@{host}/%23coder-notifications)
```

- **Bare mapping (no `?:src=dst` remap):** Coder's fixed payload already
  carries top-level `title` + `body` — which ARE apprise field names — so
  the sink binds them with zero remapping. The shorter-title remap option
  was considered and rejected: Coder's titles (e.g.
  `Workspace "my-workspace" deleted`) are already concise subjects.
- The fixed payload shape (`_version`, `msg_id`, `payload{...}`, `title`,
  `body`): only `title`/`body` feed apprise; the rest (notification name,
  user labels, CTA actions) is ignored by the sink.
- `?format=text` — Coder renders plain-text bodies; markdown/html would
  attach a second fallback body for no benefit.
- `?type=info` pinned — Coder has no severity the sink could trust, so no
  dynamic type mapping.
- **NO `?tag=coder`:** the body `urls` select the target directly — no
  fallback, no server-tag filtering, no matrix-tenant leg. (Verified sink
  contract: `urls` is body-only — JSON `urls` key or form field;
  `projects/apprise-go-api/internal/server/notify.go` has NO `?urls=`
  query fallback, and the remap engine cannot constant-assign onto `urls`.)
- **Single-endpoint fan-in:** Coder exposes ONE global webhook endpoint, so
  EVERY notification event (workspace builds, deletions, template changes)
  lands in this single room. Per-event-type routing (delivery preferences)
  is a Coder Premium feature — until then this room is the unified Coder
  event log.
- **GAP (documented, not silent):** the sink reads `urls` from the POST
  BODY only, and coderd POSTs its fixed JSON body to the endpoint URL
  as-is — a query-string `urls` is NOT promoted into the body. The
  ESO-composed endpoint delivers the credential to the coder namespace
  (ownership split done), but live delivery still needs ONE follow-up:
  either (a) sink `?urls=` query support (notify.go: read
  `r.URL.Query().Get("urls")` as a fallback like tag/format/type/title),
  or (b) a tiny in-namespace injector (sidecar/proxy that moves the query
  `urls` into the POST body). Until then Coder posts 204 VISIBLE — no
  silent centralization, no dead matrix leg. Prefer (a): one-line sink
  change, covered by the existing notify_test.go table style.

## Bot bootstrap chain (zitadel-style — manual runbook is dead)

```text
vault matrix/tuwunel-registration-secret (ONE first-seed per env)
  -> ES tuwunel-registration-secret (proton-pass ClusterSecretStore)
  -> Secret tuwunel-registration-secret (key shared-secret)
  -> TWICE: (1) tuwunel Deployment mount
       (TUWUNEL_REGISTRATION_SHARED_SECRET_FILE=/etc/tuwunel-registration/shared-secret)
     + (2) matrix-bot-bootstrap Job env REGISTRATION_SHARED_SECRET
  -> Job: GET nonce -> HMAC-SHA1 register (POST /_synapse/admin/v1/register)
     -> login (POST /_matrix/client/v3/login) -> token
  -> KEPT Secret matrix-bot-bootstrap-outputs (no ownerRef — outlives Job)
     keys: homeserver_url/access_token/user_id (rooms varsFrom) +
           notifier-token/homeserver-host (apprise ES) + url/id aliases
  -> rooms.yaml Terraform CRs (same-namespace varsFrom, NO ESO hop)
  -> apprise-stateless-urls ES (in-cluster tuwunel-k8s store, NO vault hop)
```

Retired vault paths (do NOT reseed): `matrix-rooms/homeserver-url`,
`matrix-rooms/bot-access-token`, `matrix-rooms/bot-user-id`,
`matrix-rooms/notifier-bot-token`, `matrix-rooms/homeserver-host`,
`coder/matrix-bot-token`, `coder/matrix-host` (coder's `matrix-notify` ES
now reads the bootstrapped `notifier-token`/`homeserver-host` from the kept
Secret cross-namespace via the coder-owned handoff — narrow
`coder-matrix-handoff-reader` Role/Binding in ns `matrix` + `coder-matrix`
SecretStore, owned by the coder component; old vault entries may stay as
rollback, NOT referenced). Live vault path:
`matrix/tuwunel-registration-secret` (see VAULT-SEEDS.md). ONE bot per
env (`@apprise-dev` dev / `@apprise` prd) shared by all 3 rooms — same as
the retired manual path. Rerun = delete Job + reconcile (M_USER_IN_USE
path fails LOUD with the recovery step; see matrix-bot-bootstrap.yaml).

## E2EE-vs-plaintext decision: PLAINTEXT for notifier rooms

- All four purpose rooms are `encryption_enabled: false` (live rooms in
  `rooms.yaml` + team skeleton in `terraform/examples/`: all carry
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
room-ID `!...` targets need no encoding). No secrets in Git: the bot token
+ homeserver host flow from the bootstrap kept Secret (rooms via
`varsFrom`, fallback via the in-cluster `tuwunel-k8s` ES); coder's token
flows from the same kept Secret (`notifier-token`/`homeserver-host`) via the
coder-owned handoff (SecretStore `coder-matrix` remoteNamespace `matrix` +
Role/Binding `coder-matrix-handoff-reader` in ns `matrix`, owned by the coder
component) into coder ES `matrix-notify` — vault `coder/matrix-*` retired.

## Deferred (explicitly NOT this change)

- Zitadel SMTP stays on the future mail relay — NOT apprise (email needs
  SMTP credentials per recipient domain; apprise `email://` URLs would
  spray vault-held mail passwords into request bodies).
- Prometheus `AlertmanagerConfig` / `PrometheusRule` reserved until the
  monitoring stack lands (Flux Alert CRs here cover Flux-native sources
  only).
- Proton Pass re-check: the bootstrapped `notifier-token` (consumed by coder
  too, via the coder-owned handoff — vault `coder/matrix-bot-token` retired)
  composes into an authenticated `matrixs://`
  URL (hidden-webhook class secret). Re-check vault item
  visibility/sharing before pasting anywhere; room aliases are public,
  tokens never are.
- `generic-hmac` parity (`SECRET_KEY(_FILE)` + `X-Signature` verification)
  only if apprise gains signature verification — today the header would
  be silently ignored.
- Workload placement: the apprise-go-api Deployment/Service/HPA/VPA now
  live in the infra `apprise-go-api` tenant
  (`flux/infra/components/apprise-go-api`, credential-free); this tenant
  keeps only the consumer-owned `apprise-stateless-urls` fallback (3 legs:
  flux/tofu/team — coder decoupled) + Provider/Alert wiring. Per-request
  `urls` injection is LIVE for coder (first caller); dropping the
  STATELESS_URLS fallback entirely for flux/tofu/team is follow-up work —
  the sink already supports body `urls` (missing `urls` -> 204, no crash);
  the remaining gap is coder's fixed-payload body (see the Coder GAP note:
  sink `?urls=` query support or an injector sidecar).
- Fleet tenants/workflows onboarding (`tenants/apps.yaml` + image-update
  policies + `flux-apps-push.yaml` matrix): separate task.

## End-to-end test plan (plan-only — no cluster access here)

1. Preconditions (kubectl, plan-only): `kubectl get providers,alerts -A`
   shows `apprise-*` Ready=True (4 Providers — no apprise-coder);
   `kubectl get externalsecret
   apprise-stateless-urls -n <tenant>` Synced (tuwunel-k8s store);
   `kubectl get job matrix-bot-bootstrap -n matrix` Complete +
   `matrix-bot-bootstrap-outputs` Secret exists (7 keys);
   `flux-notifications-outputs` / `tofu-runs-outputs` /
   `coder-notifications-outputs` Secrets exist (room IDs minted).
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
