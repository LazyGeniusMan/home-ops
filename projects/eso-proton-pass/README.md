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

Sample PromQL: request rate and p95 latency by route, `eso_proton_pass_build_info`.

## Develop

```sh
flox activate
cd projects/eso-proton-pass
go build ./...
go vet ./...
go test -race -shuffle=on ./...
golangci-lint run ./...
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
| `PROTON_PASS_SESSION_DIR` | no | per-OS `pass-cli` default | Session dir override; appended to the exec env only when set. |
| `LISTEN_ADDR` | no | `:8080` | HTTP listen address. |
| `PASS_CLI_BIN` | no | `pass-cli` | Path to the `pass-cli` binary (`/usr/local/bin/pass-cli` in the image). |
| `PASS_CLI_TIMEOUT` | no | `60s` | Per-invocation `pass-cli` timeout (must be a positive duration). |

Startup fails fast with `missing required env PROTON_PASS_PAT_FILE …` when the
required `*_FILE` secret is absent — no default secrets.

K8s probes (single listener on `:8080`, configurable via `LISTEN_ADDR`):

```yaml
livenessProbe:
  httpGet: {path: /healthz, port: 8080}
readinessProbe:
  httpGet: {path: /readyz, port: 8080}
```

Shutdown: `signal.NotifyContext` (SIGINT/SIGTERM) + `http.Server.Shutdown`
bounded at 10s, draining in-flight requests (`shutting down` / `drained`
logs).

## Telemetry-off evidence

`PROTON_PASS_DISABLE_TELEMETRY=1` on every exec (`baseEnv`), as image
`ENV`, and asserted in tests — plus `PASS_LOG_LEVEL=off` and
`PROTON_PASS_KEY_PROVIDER=fs` on both layers.

## Image

Published to `ghcr.io/lazygeniusman/home-ops/projects/eso-proton-pass`:

- `main` push → `:dev` (+ `:dev-<sha>`)
- tag `eso-proton-pass-v*` → `:stable` + version (e.g. `:1.2.3`); no `:latest`

Consumed in Flux via `{"$imagepolicy": "infra:eso-proton-pass:tag"}` in
`flux/infra/components/external-secrets/configs/base/eso-proton-pass-webhook.yaml`
(policy `flux/infra/update-policies/external-secrets.yaml`).

Multi-stage build: `golang:1.26.7` (digest-pinned, `CGO_ENABLED=0`) +
`pass-cli` 2.3.3 (per-arch SHA-256 verified) → `distroless/base-debian13`
(digest-pinned, no shell) as `nonroot:nonroot` (65532), exposing 8080.
