# apprise-go-api

Stateless-only Go port of Python `apprise-api`, backed by
[`apprise-go`](https://github.com/unraid/apprise-go) (v0.3.3).

Scope: stateless `POST /notify`, request-scoped attachments, and third-party
webhook remap/callback. There is **no persistent storage**
(`APPRISE_STATELESS_STORAGE=no`); stateful endpoints are unsupported.

## Layout

```text
cmd/apprise-go-api/main.go   # thin: slog → config.Load → wire → serve + Shutdown(10s)
internal/config/             # env-only config (G5 stateless subset)
internal/remap/              # ':' webhook payload mapper (G4)
internal/attach/             # SSRF policy + temp-file staging (G3)
internal/notify/             # thin wrapper over apprise-go AddAll+Send (G2)
internal/server/             # mux, handlers, writeJSON/handleMetrics helpers
```

## Config

Env-only; secrets via `*_FILE` (e.g. `SECRET_KEY_FILE`). Stateless-only
subset — see `internal/config/config.go`. `APPRISE_PLUGIN_PATHS` is accepted
but unsupported (apprise-go has no dynamic plugin loading).

## Checks

```sh
go vet ./... && go build ./... && go test ./...
```
