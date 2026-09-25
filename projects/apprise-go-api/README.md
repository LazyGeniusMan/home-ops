# apprise-go-api

Stateless-only Go port of Python `apprise-api`, backed by `apprise-go`.
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

Form posts (`curl -d`, urlencoded) work the same way.

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

Tag grammar (string form): `,`/`|` separate **OR** groups; whitespace,
`&`, `+` separate **AND** tokens. Tokens are `[priority:]name[:retry]`
(`400` on mismatch). List-form tags skip validation — one single-token OR
group each. `all` matches every target; a URL's own `?tag=` values count
as its server tags.

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

Health plus attach/config-lock flags (`status`, `version`,
`stateful_mode`, `stateless_storage`, `attach_dir`, `can_write_attach`;
see the status handler). Always JSON. `attach_dir` resolves
`APPRISE_ATTACH_DIR` (default OS temp dir); the writability probe is
TTL-cached (30s, shared with `/readyz`/`/metrics`) and failure sets
`can_write_attach: false` + `attach_permission_issue`. Non-GET → `405`.

### `GET /details`

Service catalog (`version`, `service_count`, `services`, `routes`;
see the details handler). Always JSON. Non-GET → `405`.
```

### `GET /healthz` and `GET /readyz`

`GET /healthz`: static `{"status":"ok"}`, zero downstream calls.
`GET /readyz`: attach-dir writability (TTL-cached) → `{"status":"ok"}` or
`503 {"status":"not_ready","failing":"attach-dir"}`.

K8s probes (single listener on `:8080`):

```yaml
livenessProbe:
  httpGet: {path: /healthz, port: 8080}
readinessProbe:
  httpGet: {path: /readyz, port: 8080}
```

Lifecycle: `docker stop` (SIGTERM) drains the listener gracefully
(`signal.NotifyContext` + `http.Server.Shutdown`, 10s bound); logs show
`shutting down` then `drained`.

### `GET /metrics`

Prometheus exposition via `client_golang` on the default registry
(includes `go_*` / `process_*`): `apprise_go_api_up`,
`apprise_go_api_build_info{version}`, `apprise_go_api_attach_writable`,
`apprise_go_api_supported_services`, plus per-request
`apprise_go_api_http_requests_total` /
`apprise_go_api_http_request_duration_seconds_bucket` by
`method`/`route`/`status` (`route` = matched mux pattern, never raw
path) and one slog line; exact names live in the metrics handler.
Version is `internal/version.Version` (`dev` locally; release images
inject via `ARG VERSION` ldflags). The attach probe is TTL-cached
(30s), shared by `/status`, `/readyz`, `/metrics`.

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

The reserved token **`internal`** (opt-in, never default) DNS-resolves each
host and blocks loopback, private, link-local, reserved, unspecified,
multicast, and CGN addresses (incl. alternate IP encodings). Unresolvable
hosts are blocked.

### Zero-persistence guarantee

Staged files live under `APPRISE_ATTACH_DIR` (default `os.TempDir()`) as
`apprise-attach-*` temp files, removed via deferred `CleanupAll` at
request end; partial failures clean up already-staged files first.

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
bounded by `APPRISE_WEBHOOK_MAPPING_MAX_DEPTH` (default `5`; over-depth →
`400`). Nested sources with an empty/non-mappable target are silent no-ops.

Example (flat rename `{subject, payload}` → `{title, body}`):

```sh
curl -X POST 'http://localhost:8080/notify/?:subject=title&:payload=body' \
  -d 'urls=json://localhost' -d 'subject=Deploy' -d 'payload=Done'
```

### Outbound result hook (`APPRISE_WEBHOOK_URL`)

After every notify, the result is POSTed (best-effort; transport errors
are logged, never surfaced to the notify caller) as JSON
`{source, status, output}` (`source` = client address, `status` = `0`
success / `1` delivery failure, `output` = notify response detail).
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
| `APPRISE_ATTACH_REJECT_URL` | — (empty disables denials) | `127.0.* localhost*` available as `DefaultAttachRejectURL`, not applied at load |
| `APPRISE_WEBHOOK_MAPPING_MAX_DEPTH` | `5` | `:` remap depth cap (must be positive) |
| `APPRISE_WEBHOOK_URL` | — | Outbound result callback (empty = disabled) |
| `APPRISE_PLUGIN_PATHS` | — | **Documented no-op**: accepted but unsupported — Go has no dynamic plugin loading |
| `APPRISE_DENY_SERVICES` | — | Block services by name/prefix (comma/whitespace separated) |
| `APPRISE_ALLOW_SERVICES` | — | Exclusive allowlist; non-empty wins over deny |
| `APPRISE_RECURSION_MAX` | `1` | Cap for `X-Apprise-Recursion-Count` (non-negative) |
| `APPRISE_INTERPRET_EMOJIS` | `false` | Emoji shortcode expansion |
| `APPRISE_HTTP_REDIRECTS` | `true` | Follow HTTP redirects |

**Explicitly absent (stateful-only):** upstream persistence knobs are
not read; non-`disabled` `APPRISE_STATEFUL_MODE` or non-`no`
`APPRISE_STATELESS_STORAGE` fails startup.

## Error / status-code table

| Code | When | Python-parity notes |
|---|---|---|
| `200` | All targets accepted | Body negotiated (JSON / HTML / text) |
| `204` | No valid target URLs survived validation/policy | `"There was no valid URLs provided to notify"` (upstream wording preserved) |
| `400` | Invalid JSON; empty/unknown form; remap failure; bad tag; bad attachment/SSRF/oversize file; minimum-requirements failure; bad format; bad recursion | Bodies use the fixed upstream literals |
| `405` | Non-POST on `/notify{,/}`; non-GET on `/status`, `/details` | `Allow` header set |
| `406` | Recursion count over max | Upstream quirk (406, not 405) |
| `424` | At least one target failed delivery | Partial failures surface as errors, not partial 200s |
| `431` | JSON body over `APPRISE_UPLOAD_MAX_MEMORY_SIZE` | Upstream `to large` typo preserved verbatim |
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

## Develop

```sh
flox activate
cd projects/apprise-go-api
go build ./...
go vet ./...
go test -race -shuffle=on ./...
golangci-lint run ./...
govulncheck ./...
gofmt -s -l .
```

Metric names are defined in code (see the metrics handler).

Image `ghcr.io/lazygeniusman/home-ops/projects/apprise-go-api`: `main`
push → `:dev` (+ `:dev-<sha>`), tag `apprise-go-api-v*` → `:stable` +
version; no `:latest`. Consumed in Flux via
`{"$imagepolicy": "infra:apprise-go-api:tag"}` in
`flux/infra/components/apprise-go-api/controllers/base/apprise-go-api.yaml`
(policy `flux/infra/update-policies/apprise-go-api.yaml`). Distroless
`nonroot` on `:8080`, no volumes; `ARG VERSION` surfaces via
`apprise_go_api_build_info{version="..."}` (no `/version` endpoint).

## Divergence from upstream

Intentional differences from Python `apprise-api`:

| Area | This project behavior |
|---|---|
| Scope | Stateless-only; stateful paths are `404`, storage knobs rejected at startup |
| Recursion limit | Same `406` preserved (upstream quirk) |
| Oversize body message | Same `to large` typo preserved verbatim |
| Response logs | Records synthesized server-side as `[level, date, message]` |
| Tag matching | `all` matches everything; other tokens match only URL `?tag=` values |
| Form `urls` length | Same 1024-char cap; JSON path bypasses it |
| `APPRISE_PLUGIN_PATHS` | Accepted no-op (no dynamic plugin loading in Go) |
| Outbound webhook | Best-effort POST of `{source, status, output}`; failures logged only |
