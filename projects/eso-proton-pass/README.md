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
secret with `result.jsonPath: "$.value"`.

Errors are non-2xx with a `{"error": "…"}` body: 400 invalid key /
malformed body, 404 not found (lets ESO apply the deletionPolicy), 422
unprocessable reference, 502 transient backend failure, 501 push
(pull-only), 500 fallback. Backend detail stays server-side (logged once);
envelopes carry short lowercase reasons only. Only `vault/item` is logged;
URIs redact to `pass://vault/item/<field>`.

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

Exact metric names live in `internal/server/server.go`.

## Develop

```sh
flox activate
cd projects/eso-proton-pass
go build ./...
go vet ./...
go test -race -shuffle=on ./...
golangci-lint run ./...
govulncheck ./...
gofmt -s -l .
```

## Dependencies

`go.mod` requires `github.com/prometheus/client_golang` (metrics +
`go_*`/`process_*` collectors) — `go.sum` is committed. Everything else is
stdlib (`net/http`, `os/exec`, `crypto/rand`, `log/slog`, …).

## Environment

| Variable | Required | Default | Purpose |
| -------- | -------- | ------- | ------- |
| `PROTON_PASS_PAT_FILE` | yes | — | Path to a file holding the Proton Pass PAT (never from an env value). |
| `PROTON_PASS_AGENT_REASON` | no (auto) | fresh value per exec | Unique per `pass-cli` exec for audit attribution. |
| `PROTON_PASS_DISABLE_TELEMETRY` | forced `1` | `1` | Set on every exec and as container `ENV`. |
| `PROTON_PASS_KEY_PROVIDER` | forced `fs` | `fs` | Filesystem key storage (no kernel keyring in containers). |
| `PASS_LOG_LEVEL` | forced `off` | `off` | pass-cli log level (forced off on every exec and as container `ENV`). |
| `PROTON_PASS_SESSION_DIR` | no | per-OS `pass-cli` default | Session dir override; appended to the exec env only when set. |
| `LISTEN_ADDR` | no | `:8080` | HTTP listen address. |
| `PASS_CLI_BIN` | no | `pass-cli` | Path to the `pass-cli` binary (`/usr/local/bin/pass-cli` in the image). |
| `PASS_CLI_TIMEOUT` | no | `60s` | Per-invocation `pass-cli` timeout (must be a positive duration). |

Startup fails fast with `missing required env PROTON_PASS_PAT_FILE …` when the
required `*_FILE` secret is absent — no default secrets.

Probes: `/healthz` (liveness) and `/readyz` (readiness) on `:8080`
(`LISTEN_ADDR`). Shutdown drains in-flight requests (10s bound).

## Telemetry-off evidence

Telemetry off on every exec, as image `ENV`, and asserted in tests
(`PASS_LOG_LEVEL=off`, `PROTON_PASS_KEY_PROVIDER=fs` on both layers).

## Image

Published to `ghcr.io/lazygeniusman/home-ops/projects/eso-proton-pass`:

- `main` push → `:dev` (+ `:dev-<sha>`)
- tag `eso-proton-pass-v*` → `:stable` + version (e.g. `:1.2.3`); no `:latest`

Consumed in Flux via `{"$imagepolicy": "infra:eso-proton-pass:tag"}` in
`flux/infra/components/external-secrets/configs/base/eso-proton-pass-webhook.yaml`
(policy `flux/infra/update-policies/external-secrets.yaml`).

Multi-stage build: `golang:1.26.7` + `pass-cli` 2.3.3 →
`distroless/base-debian13` (nonroot 65532), exposing 8080; see the
Dockerfile for pins.
