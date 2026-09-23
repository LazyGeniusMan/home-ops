# external-dns-netbird

ExternalDNS webhook provider backed by NetBird DNS Custom Zones. It runs as
a localhost-only sidecar next to ExternalDNS (`--provider=webhook`) and
translates the webhook API (`/`, `/records`, `/adjustendpoints`) into
NetBird Public API calls (`/api/dns/zones`, `/api/dns/zones/{zoneId}/records`).

No telemetry is collected or transmitted. The only network traffic is NetBird
Public API calls plus the local webhook, health, and metrics listeners.

## Configuration (environment)

| Variable           | Required | Default             | Description                                              |
| ------------------ | -------- | ------------------- | -------------------------------------------------------- |
| `NETBIRD_PAT_FILE` | yes      | —                   | Path to a file holding the NetBird personal access token |
| `NETBIRD_BASE_URL` | no       | `https://api.netbird.io` | NetBird Public API base URL (self-hosted override)  |
| `DOMAIN_FILTER`    | no       | (all zones)         | Comma-separated domain allow-list                        |
| `WEBHOOK_ADDR`     | no       | `127.0.0.1:8888`    | Listen address for the webhook API (keep localhost-only) |
| `METRICS_ADDR`     | no       | `:8080`             | Listen address for `/healthz` and `/metrics`             |
| `DEFAULT_TTL`      | no       | `300`               | TTL applied to endpoints without an explicit TTL         |
| `LOG_LEVEL`        | no       | `info`              | JSON log level (`debug`, `info`, `warn`, `error`)        |

The PAT is read from file content so it can be mounted from a Kubernetes
secret (or ESO `SecretStore`) without ever appearing in env or args.

## Record mapping

- One ExternalDNS endpoint (`DNSName` + `RecordType`) maps to one NetBird
  record entry per target, since the NetBird records API stores a single
  `content` value per entry. `Records` groups entries back into endpoints.
- Supported types: `A`, `AAAA`, `CNAME` (the NetBird records API set).
  Other types are dropped by `AdjustEndpoints`, which also normalizes case
  and fills missing TTLs with `DEFAULT_TTL` so `Records`/`AdjustEndpoints`
  stay in parity and the planner sees no spurious diffs.

## Zone auto-creation

- On create/update, the provider resolves the longest-suffix NetBird zone
  matching the endpoint name. When no zone matches, it auto-creates the
  zone (`POST /api/dns/zones`) with the longest `DOMAIN_FILTER` entry that
  is a suffix of the name (falling back to the immediate parent domain when
  no filter is configured) and retries the record creation in the same
  apply — Gateway/Service records self-heal without manual zone setup.
- The existing zone list is re-checked before creation, so concurrent
  applies stay idempotent (no duplicate zones). Names outside
  `DOMAIN_FILTER` are rejected with a permanent error (no zone created);
  NetBird API failures (including `429`/`5xx`) surface as soft errors so
  ExternalDNS retries the apply.

## Endpoints

| Listener     | Route               | Description                              |
| ------------ | ------------------- | ---------------------------------------- |
| webhook      | `GET /`             | Negotiate: returns the domain filter     |
| webhook      | `GET /records`      | Current endpoints                        |
| webhook      | `POST /records`     | Apply planned changes (`204` on success) |
| webhook      | `POST /adjustendpoints` | Provider-specific adjustment         |
| ops          | `GET /healthz`      | Liveness: `{"status":"ok"}`, zero downstream calls |
| ops          | `GET /readyz`       | Readiness: NetBird API probe (`200` up, `503 {"status":"not_ready","failing":"netbird-api"}` down) |
| ops          | `GET /version`      | Release version (`internal/version.Version`, `dev` unless ldflags-injected) |
| ops          | `GET /metrics`      | Prometheus metrics (text exposition, incl. `go_*`/`process_*`; domain: `external_dns_netbird_records_errors_total`, `external_dns_netbird_apply_changes_errors_total`, `external_dns_netbird_adjust_endpoints_errors_total`, `external_dns_netbird_build_info{version}`) |

Error contract (`internal/server/errors.go`): transient NetBird failures
(soft errors via `provider.NewSoftError`, `%w`-wrapped, lowercase)
surface as `502` (ExternalDNS retries); permanent failures (e.g.
`ErrNoMatchingZone`) surface as `422`. Malformed payloads are `400`.
Anything unmapped is `500`. Handlers log the full error chain once
server-side and return only a sanitized `{"error"}` envelope with no
traces, tokens, or paths.

Lifecycle: `docker stop` (SIGTERM) drains both listeners gracefully
(`signal.NotifyContext` + `http.Server.Shutdown(10s)`); logs show
`shutting down` then `drained`.

K8s probes (ops listener on `:8080` via `METRICS_ADDR`; webhook API stays
localhost-only on `127.0.0.1:8888` via `WEBHOOK_ADDR`):

```yaml
livenessProbe:
  httpGet: {path: /healthz, port: 8080}
readinessProbe:
  httpGet: {path: /readyz, port: 8080}
```

Sample PromQL: `external_dns_netbird_build_info`,
`rate(external_dns_netbird_records_errors_total[5m])`.

## Develop

```sh
flox activate
cd projects/external-dns-netbird
go build ./...
go vet ./...
go test -race -shuffle=on ./...
golangci-lint run ./...
gofmt -s -l .
docker build --build-arg VERSION=1.2.3 -t external-dns-netbird:dev .
```

The Docker `ARG VERSION` is wired into
`-ldflags "-X .../internal/version.Version=$VERSION"` and surfaces via
`GET /version` and `external_dns_netbird_build_info{version="..."}`.

Image: `ghcr.io/lazygeniusman/home-ops/projects/external-dns-netbird` (`:dev`
+ `:dev-<sha>` on any branch push, `:stable` + version on
`external-dns-netbird-v*` tags; neither leg publishes `:latest`).
<!-- ci-trigger: force external-dns-netbird workflow on push -->
<!-- ci-trigger-2: verify branches:main fix -->
