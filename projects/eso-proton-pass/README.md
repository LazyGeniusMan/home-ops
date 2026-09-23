# eso-proton-pass

External Secrets Operator (ESO) webhook provider for Proton Pass (pull-only).
A Go HTTP service that resolves secrets via the `pass-cli` backend. ESO's
generic webhook provider issues GET (and HEAD for Validate) requests; push
operations are not implemented.

## Webhook contract

Secret addressing (validated by `internal/provider.ValidateKey`):

```text
pass://{vault}/{item}/{field}
```

Routes (see `internal/server/server.go`):

| Method | Path      | Purpose                                              |
| ------ | --------- | ---------------------------------------------------- |
| GET    | `/get`    | Pull: `?key=pass://vault/item/field` → `{"value"}`   |
| POST   | `/get`    | Pull (JSON body variant): `{"remoteRef":{"key":"…"}}` |
| HEAD   | `/`       | Validate path (ESO `Validate`) → 200                 |
| GET    | `/`       | Validate path → 200                                  |
| GET    | `/healthz` | Liveness → `{"status":"ok"}` (zero downstream calls) |
| GET    | `/readyz` | Readiness: 200 `{"status":"ok"}`, or 503 `{"status":"not_ready","failing":"pass-cli"}` when the backend is unreachable |
| GET    | `/metrics` | Prometheus metrics (text exposition, incl. `go_*`/`process_*`) |
| POST   | `/push`   | → 501 (pull-only, not implemented)                   |

All pull responses use the `{"value": "…"}` envelope, so ESO extracts the
secret with `result.jsonPath: "$.value"`. Templated ESO usage:

```yaml
provider:
  webhook:
    url: http://eso-proton-pass:8080
    result:
      jsonPath: $.value
    secrets:
      - name: db-password
        remoteRef:
          key: pass://prod-vault/postgres/password
```

which the provider serves as `GET /get?key=pass://prod-vault/postgres/password`
→ `200 {"value": "<secret>"}`.

Errors are returned as non-2xx with a `{"error": "…"}` body. Mapping
(`internal/server/errors.go`, apprise `StatusError` + `StatusCodeOf` shape):
400 invalid key / malformed body, 404 secret not found (lets ESO apply the
ExternalSecret deletionPolicy), 422 unprocessable reference, 502 transient
backend failure, 501 push (pull-only), 500 fallback. Backend detail stays
server-side (logged once): envelopes carry only short lowercase reasons with
no traces, tokens, or paths. The field name is never included in logs or
errors — only `vault/item` is logged (`vaultItem` helper) and URIs are
redacted to `pass://vault/item/<field>` in error strings.

## Metrics and version

`GET /metrics` (via `promhttp` on the default registry) exposes:

- `eso_proton_pass_http_requests_total{method,route,status}` — route is the
  registered mux pattern, never the raw path, so cardinality stays bounded.
- `eso_proton_pass_http_request_duration_seconds{method,route}`
  (`DefBuckets`).
- `eso_proton_pass_up` (`== 1` while serving).
- `eso_proton_pass_build_info{version="…"}` — the `internal/version.Version`
  string (`"dev"` for local builds, overridden by ldflags
  `-X .../internal/version.Version=$VERSION`; the Dockerfile wires
  `ARG VERSION` through exactly this flag).
- Standard `go_*` / `process_*` runtime series.

Sample PromQL: `sum by (route)
(rate(eso_proton_pass_http_requests_total[5m]))`,
`eso_proton_pass_build_info`.

## Dependencies

`go.mod` requires `github.com/prometheus/client_golang` (metrics +
`go_*`/`process_*` collectors) — `go.sum` is committed. Everything else is
stdlib (`net/http`, `os/exec`, `crypto/rand`, `log/slog`, …).

## Environment

| Variable | Required | Default | Purpose |
| -------- | -------- | ------- | ------- |
| `PROTON_PASS_PAT_FILE` | yes | — | Path to a file whose **content** is the Proton Pass personal access token. The token is never taken from an env value directly; it is read from this file at startup and passed to `pass-cli login` via the `PROTON_PASS_PERSONAL_ACCESS_TOKEN` env var on a private, per-process environment. |
| `PROTON_PASS_AGENT_REASON` | no (auto) | fresh value per exec | A fresh, unique reason is generated for **every** `pass-cli` exec (`NewAgentReason`: caller prefix + random 8-byte hex suffix), so audit logs can distinguish invocations. |
| `PROTON_PASS_DISABLE_TELEMETRY` | forced `1` | `1` | Set on **every** `pass-cli` exec (`baseEnv`) and as a container `ENV` in the Dockerfile. |
| `PROTON_PASS_KEY_PROVIDER` | forced `fs` | `fs` | Filesystem key storage scoped to the session dir — containers cannot access the kernel keyring. Applied to every `pass-cli` exec and as a container `ENV`. |
| `PROTON_PASS_SESSION_DIR` | no | per-OS `pass-cli` default | Session dir override; appended to the exec env only when set. |
| `LISTEN_ADDR` | no | `:8080` | HTTP listen address. |
| `PASS_CLI_BIN` | no | `pass-cli` | Path to the `pass-cli` binary (`/usr/local/bin/pass-cli` in the image). |
| `PASS_CLI_TIMEOUT` | no | `60s` | Per-invocation `pass-cli` timeout (must be a positive duration). |

Startup fails fast with `missing required env PROTON_PASS_PAT_FILE …` when the
required `*_FILE` secret is absent — no default secrets.

Shutdown: `signal.NotifyContext` (SIGINT/SIGTERM) + `http.Server.Shutdown`
bounded at 10s, draining in-flight requests (`shutting down` / `drained`
logs).

## Telemetry-off evidence

Three independent layers, all asserting `PROTON_PASS_DISABLE_TELEMETRY=1`:

1. **Exec env** — `internal/passclient/passclient.go` `baseEnv()` applies
   `PROTON_PASS_DISABLE_TELEMETRY=1` (plus `PASS_LOG_LEVEL=off`,
   `PROTON_PASS_KEY_PROVIDER=fs`, `PROTON_PASS_LINUX_KEYRING=kernel`) to
   every `pass-cli` invocation.
2. **Image env** — the `Dockerfile` sets
   `ENV PROTON_PASS_DISABLE_TELEMETRY=1` (plus `PASS_LOG_LEVEL=off`,
   `PROTON_PASS_KEY_PROVIDER=fs`) so even a stray non-`baseEnv` exec inherits
   telemetry-off.
3. **Test** — `internal/passclient/passclient_test.go` asserts the hardened
   env vars are present on the exec environment.

## Image

Published to `ghcr.io/lazygeniusman/home-ops/projects/eso-proton-pass` by
the self-contained `.github/workflows/eso-proton-pass.yml` (no local
actions):

- any branch push → `:dev` (+ `:dev-<sha>`)
- tag `eso-proton-pass-v*` → `:stable` plus the stripped version
  (e.g. `eso-proton-pass-v1.2.3` → `:1.2.3`); neither leg publishes
  `:latest`

Multi-stage build: pinned `golang:1.26.7` toolchain (digest-pinned) with
`CGO_ENABLED=0`, downloading pinned `pass-cli` 2.3.3 (per-arch SHA-256
verified) → pinned `distroless/base-debian13` runtime (digest-pinned, no
shell), running as the non-root `nonroot:nonroot` user (65532), exposing 8080.
<!-- ci-trigger: no-op to exercise eso-proton-pass workflow on push -->
<!-- ci-trigger-2: verify branches:main fix -->
