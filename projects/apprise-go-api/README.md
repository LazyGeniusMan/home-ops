# apprise-go-api

Stateless-only Go port of Python `apprise-api`, backed by
[`apprise-go`](https://github.com/unraid/apprise-go).

Upstream references (read-only): `/tmp/home-ops-docs/apprise-go-docs`
(library contract), `/tmp/home-ops-docs/apprise-api-py-docs` (Python API
parity), `/tmp/home-ops-docs/apprise-docs` (notification-schema syntax).

Send notifications to dozens of services with a single HTTP call — no
accounts, no database, no persistent state. Every request carries its own
target URLs; nothing is stored between requests.

## Scope: stateless-only

This service implements **only** the stateless notification path:

| Supported | Not supported (stateful) |
|---|---|
| `POST /notify`, `POST /notify/` | Keyed routes such as `/notify/{KEY}` |
| `GET /status`, `/details`, `/metrics`, `/healthz`, `/readyz` | `/add/`, `/del/`, `/cfg/`, `/json/...`, `/xml/...` |
| Request-scoped attachments | Any persistent config storage |
| Third-party webhook remap (`?:src=dst`) + result callback | Per-key management UI / API |

Any path other than the seven routes above returns **404**
(`APPRISE_STATELESS_STORAGE=no`; persistence is unsupported by design).
`APPRISE_STATEFUL_MODE` must be `"disabled"` — any other value fails
startup.

## Quickstart

```sh
# Build and run (listens on :8080)
docker build -t apprise-go-api ./projects/apprise-go-api
docker run --rm -p 8080:8080 apprise-go-api
```

```sh
# Send a notification (JSON)
curl -X POST http://localhost:8080/notify \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -d '{
    "urls": "json://localhost",
    "body": "Backup finished",
    "title": "Nightly job",
    "type": "success",
    "format": "text"
  }'
# {"error":null,"details":[["INFO","2026-09-21 07:00:00","Delivered Stateless Notification(s)"]]}
```

```sh
# Same thing as a form post (curl -d defaults to urlencoded)
curl -X POST http://localhost:8080/notify \
  -d 'urls=json://localhost' \
  -d 'body=Backup finished' \
  -d 'title=Nightly job' \
  -d 'type=success'
```

## Endpoint reference

### `POST /notify` and `POST /notify/`

Both paths behave identically (POST-only; any other method → `405` with
`Allow: POST`). Three content types are accepted, detected from
`Content-Type`:

| Content type | Detection | Notes |
|---|---|---|
| JSON | `Content-Type` matches `(text\|application)/(x-)?json` | `urls`/`tag` accept string or list; `attach` accepts string, list, or dict |
| `multipart/form-data` | `multipart/form-data` | File parts (any field name) become attachments |
| `application/x-www-form-urlencoded` | everything else | First value wins for scalars |

#### Fields

| Field | Type | Default / fallback | Notes |
|---|---|---|---|
| `urls` | string or list | `APPRISE_STATELESS_URLS` | Split on commas/whitespace. Form values over 1024 chars are dropped |
| `body` | string | — | Required unless attachments are present |
| `title` | string | `?title=` query fallback | Optional |
| `type` | `info\|success\|warning\|failure` | `info` (`?type=` fallback) | Anything else → 400 |
| `format` | `text\|markdown\|html` | `text` (`?format=` fallback) | Anything else → 400 |
| `tag` / `tags` | string or list | `?tag=` / `?tags=` fallback | `tag` wins over `tags`. See tag grammar below |
| `attach` / `attachment` / `attachments` | string, list, or dict | — | `attach` wins; see [Attachments](#attachments) |

Tag grammar (string form): `,` and `|` separate **OR** groups;
whitespace, `&`, `+` separate **AND** tokens within a group. Tokens are
`[priority:]name[:retry]`, validated against `^[a-z0-9\s|, _:+&-]+$`
(`400 "Unsupported characters found in tag definition"` on mismatch).
List-form tags skip grammar validation — each entry becomes its own
single-token OR group. The special token `all` matches every target; a
target URL's own `?tag=` query values count as its server tags.

#### Headers

| Header | Behavior |
|---|---|
| `X-Apprise-Recursion-Count` | Missing → `0`. Negative or unparseable → `400`. Over `APPRISE_RECURSION_MAX` → **`406`** (not 405) |
| `X-Apprise-ID` | Accepted and logged (no per-send identity knob in the engine) |
| `X-Apprise-Log-Level` | Must be `CRITICAL\|ERROR\|WARNING\|INFO\|DEBUG\|TRACE` (case-insensitive); unknown values fall back to the service default |
| `Accept` | `application/json` → JSON; `text/html` or `text/*` → HTML; anything else → plain text. Missing/empty `Accept` falls back to the request `Content-Type` |

#### Response shapes

Success (`200`) and delivery-failure (`424`) bodies are negotiated:

- **JSON** (`Accept: application/json`):
  `{"error": null|string, "details": [[level, date, message], ...]}` —
  records are `[level, date, message]` triples, e.g.
  `["INFO", "2026-09-21 07:00:00", "Delivered Stateless Notification(s)"]`.
- **HTML** (`Accept: text/html`): `<ul class="logs"><li class="log_INFO">…</li></ul>`.
- **Text** (default): `2026-09-21 07:00:00 [INFO] notify: …` lines.

Validation failures use the same negotiation with a plain error message
(JSON `{"error": msg}` or `text/plain`).

### `GET /status`

Health plus attach/config-lock flags. Always JSON.

```json
{
  "status": "ok",
  "version": "0.1.0",
  "stateful_mode": "disabled",
  "stateless_storage": "no",
  "attach_dir": "/tmp",
  "can_write_attach": true,
  "config_lock": false
}
```

`attach_dir` resolves `APPRISE_ATTACH_DIR`, defaulting to the OS temp dir.
A writability probe (create + remove a temp file) is TTL-cached (30s, shared
with `/readyz` and `/metrics`); failure adds
`"attach_permission_issue": "ATTACH_PERMISSION_ISSUE"` and sets
`can_write_attach: false`. Non-GET → `405`.

### `GET /details`

Service catalog: notification schemas supported by apprise-go plus the
stateless route table. Always JSON. Non-GET → `405`.

```json
{
  "version": "0.1.0",
  "stateful_mode": "disabled",
  "stateless_storage": "no",
  "service_count": 214,
  "services": ["discord", "json", "slack", "..."],
  "routes": ["POST /notify", "GET /status", "GET /details", "GET /metrics", "GET /healthz", "GET /readyz"]
}
```

### `GET /healthz` and `GET /readyz`

`GET /healthz` is the liveness probe: static `{"status":"ok"}`, zero
downstream calls, under 50ms. `GET /readyz` is the readiness probe: it
checks attach-dir writability (TTL-cached) and returns `{"status":"ok"}`,
or `503 {"status":"not_ready","failing":"attach-dir"}` when the staging
dir is not writable. `/details` stays a domain endpoint (service catalog),
never a probe.

### `GET /metrics`

Prometheus exposition via `client_golang` on the default registry
(includes `go_*` / `process_*` runtime series):

```text
apprise_go_api_up 1
apprise_go_api_build_info{version="1.2.3"} 1
apprise_go_api_attach_writable 1
apprise_go_api_supported_services 214
apprise_go_api_http_requests_total{method="GET",route="/healthz",status="200"} 1
apprise_go_api_http_request_duration_seconds_bucket{method="GET",route="/healthz",le="0.005"} 1
```

Every request is observed as `apprise_go_api_http_requests_total` /
`apprise_go_api_http_request_duration_seconds_bucket` by
`method`/`route`/`status`, where `route` is the matched mux pattern (never
the raw path), plus one slog line (`method`, `route`, `status`,
`duration`). The version comes from `internal/version.Version`
(`dev` for local builds; release images inject it with
`-X .../internal/version.Version=$VERSION` via `ARG VERSION` in the
Dockerfile, e.g. `docker build --build-arg VERSION=1.2.3`). The attach
writability probe (`MkdirAll` + `CreateTemp`) is TTL-cached (30s) and shared
by `/status`, `/readyz`, and `/metrics` — never per scrape.

## Attachments

Attachments are **request-scoped only**: staged to temp files, streamed to
the notification targets, then deleted when the request ends. There is
zero persistence — no volume is required or used.

### Three mechanisms

1. **Multipart file parts** — any form field name is accepted (mirrors
   Python's `request.FILES`). Blank filenames fall back to
   `attachment.NNN`; `application/octet-stream` parts are re-typed from
   the filename.
2. **Remote `http(s)` URLs** — plain string values are downloaded
   server-side (SSRF-checked, redirects re-checked against the policy)
   and staged. Non-web schemes are rejected.
3. **JSON dicts** — `{"base64": "<data>", "filename": "report.pdf"}` for
   inline content, or `{"url": "https://…", "filename": "…?"}` for remote
   content with an explicit name. Dicts with neither key, bad base64, or
   non-string filenames are `400`s.

The winning alias is `attach` > `attachment` > `attachments`
(form keys beat JSON keys); blank strings are ignored entries.

### Limits

| Knob | Default | Meaning |
|---|---|---|
| `APPRISE_ATTACH_SIZE` | `200` (MiB per file) | `<= 0` disables attachments entirely (any attachment content → `400`) |
| `APPRISE_MAX_ATTACHMENTS` | `6` (per request) | `0` = unlimited; over-limit → `400` |
| `APPRISE_UPLOAD_MAX_MEMORY_SIZE` | `3` (MiB body budget) | Oversize JSON body → `431` |
| filename length | 250 chars | Longer → `400` |
| multipart parse memory | 32 MiB | Spills to disk beyond this (server-internal, not configurable) |

Per-file oversize, malformed entries, SSRF denials, and fetch failures
are all `400 "Bad Attachment"`. A target that cannot carry attachments
fails the send with the staged filename attached so the failure is never
silent (`424`).

### SSRF policy (`APPRISE_ATTACH_ALLOW_URL` / `APPRISE_ATTACH_REJECT_URL`)

**DENY-first, allow-second**: a remote URL matching any deny rule is
rejected; otherwise it is fetched only if it matches an allow rule.
Rules are comma/whitespace separated; each entry may be a full URL
(`https://…`, scheme-pinned), a schemeless URL (matches both schemes), a
plain hostname/IP, or a wildcard (`*` prefix match, `?` single char).

| Knob | Default | Meaning |
|---|---|---|
| `APPRISE_ATTACH_ALLOW_URL` | `*` (allow all) | Empty means `*` |
| `APPRISE_ATTACH_REJECT_URL` | `127.0.* localhost*` | Empty disables denials |

The reserved token **`internal`** (opt-in, never default) resolves each
attachment host via DNS and blocks loopback, private, link-local,
reserved, unspecified, multicast, and CGN (`100.64.0.0/10`) addresses —
including ones reached via DNS or alternate IP encodings
(decimal/octal/hex/short IPv4 forms). A host that cannot be resolved is
treated as internal (blocked), since an unclassifiable destination cannot
be proven safe.

### Zero-persistence guarantee

Staged files live under `APPRISE_ATTACH_DIR` (or `os.TempDir()` when
unset) as `apprise-attach-*` temp files. They are removed via a deferred
`CleanupAll` at request end — never GC-dependent — and partial staging
failures clean up already-staged files before returning the error.

## Webhooks

### Inbound remap (`?:src=dst` query parameters)

Third-party webhooks can reshape their payload on the fly: any query key
starting with `:` is a remap rule applied to the decoded payload
**before** validation. Rule order follows first appearance in the raw
query string. Any mapping failure → `400 "Payload field mapping failed"`.

Mappable targets (exact, case-sensitive): `format`, `type`, `title`,
`body`, `attachment`, `tag`, `tags`, `urls`.

| Op | Form | Behavior |
|---|---|---|
| Rename | `?:payload=body` | Source value moves into the target; source key is removed |
| Swap | both sides already hold expected fields | Values are exchanged instead of overwritten |
| Delete | `?:debug=` (empty target) | Top-level key is removed; deleting a missing key is a no-op |
| Constant | `?:type=info` | Target assigned verbatim when the source names an expected field or a present payload key |

Sources may be nested paths with dot-walk and `[N]` array indexing,
bounded by `APPRISE_WEBHOOK_MAPPING_MAX_DEPTH` (default `5`; each key
lookup and each index counts as one step; over-depth → `400`).
Nested sources with an empty or non-mappable target are silent no-ops.

Documented examples:

```sh
# 1. Flat rename: third-party {subject, payload} -> {title, body}
curl -X POST 'http://localhost:8080/notify/?:subject=title&:payload=body' \
  -d 'urls=json://localhost' -d 'subject=Deploy' -d 'payload=Done'

# 2. Nested source: {event: {title: ...}} -> title
curl -X POST 'http://localhost:8080/notify/?:event.title=title' \
  -H 'Content-Type: application/json' \
  -d '{"urls":"json://localhost","event":{"title":"Alert"}}'

# 3. Array source: first item's text -> body
curl -X POST 'http://localhost:8080/notify/?:items[0].text=body' \
  -H 'Content-Type: application/json' \
  -d '{"urls":"json://localhost","items":[{"text":"Hello"}]}'

# 4. Constant + delete: pin type, drop noisy key
curl -X POST 'http://localhost:8080/notify/?:type=info&:debug=' \
  -d 'urls=json://localhost' -d 'body=Hi' -d 'debug=verbose'
```

### Outbound result hook (`APPRISE_WEBHOOK_URL`)

After every notify, the result is POSTed (best-effort; transport errors
are logged, never surfaced to the notify caller) as JSON:

```json
{"source": "10.0.0.5", "status": 0, "output": "..."}
```

`source` is the notifying client's address, `status` is `0` on success /
`1` on any delivery failure, `output` is the notify response detail.
`User-Agent: Apprise-API`, `Content-Type: application/json`.

The URL must be `http(s)` with a usable host. Embedded `user[:pass]`
becomes basic auth; query keys control delivery: `?verify=` toggles TLS
verification (default on), `?cto=` / `?rto=` set connect/read timeouts in
seconds (default `4.0` each). Remaining query keys are forwarded as
request params.

## Configuration

Env-only; secrets via `*_FILE` (e.g. `SECRET_KEY_FILE`, suitable for
Kubernetes projected volumes / ESO mounts). `SECRET_KEY` inline is also
accepted as a fallback.

| Variable | Default | Meaning |
|---|---|---|
| `HTTP_PORT` | `8080` | Listen port (`:8080`). `APPRISE_BASE_URL` host part is ignored |
| `LOG_LEVEL` | `info` | `debug\|info\|warn\|warning\|error` |
| `DEBUG` | `false` | Debug logging |
| `APPRISE_BASE_URL` | — | Public base URL (informational) |
| `ALLOWED_HOSTS` | — | Comma-separated Host-header allowlist |
| `SECRET_KEY` / `SECRET_KEY_FILE` | — | Internal callback auth (`_FILE` wins) |
| `TZ` | `UTC` | Log timestamp timezone |
| `PUID` / `PGID` | `0` | Desired runtime ownership (informational under distroless nonroot) |
| `WORKER_COUNT` | `0` | Concurrent notify fan-out bound (`0` = `GOMAXPROCS`) |
| `TIMEOUT` | `30` | Seconds bounding a single notify call |
| `APPRISE_STATEFUL_MODE` | `disabled` | Must be `disabled`; anything else fails startup |
| `APPRISE_STATELESS_URLS` | — | Fallback URLs when a request carries none |
| `APPRISE_STATELESS_STORAGE` | `no` | Must be `no`; persistence is unsupported |
| `APPRISE_ATTACH_DIR` | — | Attachment staging dir (default `os.TempDir()`) |
| `APPRISE_ATTACH_SIZE` | `200` | Per-file limit in MiB; `<= 0` disables attachments |
| `APPRISE_MAX_ATTACHMENTS` | `6` | Per-request cap; `0` = unlimited |
| `APPRISE_UPLOAD_MAX_MEMORY_SIZE` | `3` | JSON/form body budget in MiB (negative values use their magnitude); oversize → `431` |
| `APPRISE_ATTACH_ALLOW_URL` | `*` | SSRF allowlist (empty = `*`) |
| `APPRISE_ATTACH_REJECT_URL` | — | SSRF denylist (empty disables denials; Python default `127.0.* localhost*` applies when unset) |
| `APPRISE_WEBHOOK_MAPPING_MAX_DEPTH` | `5` | `:` remap depth cap (must be positive) |
| `APPRISE_WEBHOOK_URL` | — | Outbound result callback (empty = disabled) |
| `APPRISE_PLUGIN_PATHS` | — | **Documented no-op**: accepted but unsupported — Go has no dynamic plugin loading |
| `APPRISE_DENY_SERVICES` | — | Block services by name/prefix (comma/whitespace separated) |
| `APPRISE_ALLOW_SERVICES` | — | Exclusive allowlist; non-empty wins over deny |
| `APPRISE_RECURSION_MAX` | `1` | Cap for `X-Apprise-Recursion-Count` (non-negative) |
| `APPRISE_INTERPRET_EMOJIS` | `false` | Emoji shortcode expansion |
| `APPRISE_HTTP_REDIRECTS` | `true` | Follow HTTP redirects |

**Explicitly absent (stateful-only):** upstream persistence knobs —
per-key config storage, config-cache/database settings, and any other
`APPRISE_STATEFUL_*` / storage variables from Python apprise-api — are
not read. If you set `APPRISE_STATEFUL_MODE` to anything but `disabled`
or `APPRISE_STATELESS_STORAGE` to anything but `no`, the service refuses
to start rather than silently ignoring them.

## Error / status-code table

| Code | When | Python-parity notes |
|---|---|---|
| `200` | All targets accepted | Body negotiated (JSON / HTML / text) |
| `204` | No valid target URLs survived validation/policy | `"There was no valid URLs provided to notify"` (upstream wording preserved) |
| `400` | Invalid JSON (`"Invalid JSON Payload provided"`); empty/unknown form (`"Bad FORM Payload provided"`); remap failure (`"Payload field mapping failed"`); bad tag (`"Unsupported characters found in tag definition"`); bad attachment/SSRF/oversize file (`"Bad Attachment"`); minimum-requirements failure (`"Payload lacks minimum requirements"`); bad format (`"An invalid body input format was specified"`); bad recursion (`"An invalid recursion value was specified"`) | Minimum-requirements check bundles body/attach + type validation into one message, per `views.py:2016`. Bundled type defaulting (`info`) also matches upstream |
| `405` | Non-POST on `/notify{,/}`; non-GET on `/status`, `/details` | `Allow` header set |
| `406` | `X-Apprise-Recursion-Count` over `APPRISE_RECURSION_MAX` (`"The recursion limit has been reached"`) | **406, not 405** — preserves the upstream quirk |
| `424` | At least one target failed delivery (`"One or more notifications could not be sent"` + cause) | Partial failures surface as errors, not partial 200s |
| `431` | JSON body over `APPRISE_UPLOAD_MAX_MEMORY_SIZE` (`"JSON Payload provided is to large"`) | Upstream typo (`to large`) preserved verbatim |
| `404` | Any other path (keyed `/notify/{KEY}`, `/add/`, `/cfg/`, …) | Stateless-only: no per-key storage routes exist |

## Layout

```text
cmd/apprise-go-api/main.go   # thin: slog → config.Load → wire → serve + Shutdown(10s)
internal/config/             # env-only config (stateless subset)
internal/remap/              # ':' webhook payload mapper
internal/attach/             # SSRF policy + temp-file staging
internal/notify/             # thin wrapper over apprise-go AddAll+Send + outbound hook
internal/server/             # mux + routes; handler/sender/validation/errors/health/metrics/middleware split
internal/version/            # build version (dev default; ldflags ARG VERSION)
```

## Checks

```sh
go vet ./... && go build ./... && go test ./...
```

CI (`.github/workflows/apprise-go-api.yml`) runs vet + build + test +
golangci-lint on pushes to `main` touching `projects/apprise-go-api/**`,
and publishes `dev` / `stable` image tags (`apprise-go-api-v<semver>`)
to GHCR. The Dockerfile serves on port `8080` as distroless `nonroot`
with no volumes.

## Divergence from upstream

Intentional differences from Python `caronc/apprise-api`. New
differences must be recorded here (copy the template row).

| Date | Area | Upstream behavior | This project behavior | Reason |
|---|---|---|---|---|
| 2026-09-21 | Scope | Stateful keyed routes (`/notify/{KEY}`, `/cfg/`, …) with persistent storage | Stateless-only; those paths are `404`, storage knobs rejected at startup | No persistent storage in this deployment model |
| 2026-09-21 | Recursion limit | `406` on recursion over max (quirk: not `405`) | Same `406` preserved | Bug-for-bug parity |
| 2026-09-21 | Oversize body message | `"JSON Payload provided is to large"` (typo) | Same message preserved verbatim | Bug-for-bug parity |
| 2026-09-21 | Response logs | Django `LogCapture` records from apprise internals | Records synthesized server-side as `[level, date, message]` | apprise-go exposes no log-capture hook |
| 2026-09-21 | Tag matching | Servers carry configured tags; full `is_exclusive_match` | Stateless servers carry no tags; `all` matches everything, other tokens match only URL `?tag=` values | Stateless URLs have no configured tags by construction |
| 2026-09-21 | Form `urls` length | `URLS_MAX_LEN` (1024) form cap | Same 1024-char cap; JSON path bypasses it | Parity |
| 2026-09-21 | `APPRISE_PLUGIN_PATHS` | Loads custom Python plugins at runtime | Accepted, documented no-op | Go has no dynamic plugin loading |
| 2026-09-21 | Outbound webhook | `send_webhook` via requests with full template-arg handling | Best-effort POST of `{source, status, output}`; failures logged only | Keep notify path dependency-free (stdlib only) |
| YYYY-MM-DD | Area | Upstream behavior | This project behavior | Reason |
