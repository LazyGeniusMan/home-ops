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

## Endpoints

| Listener     | Route               | Description                              |
| ------------ | ------------------- | ---------------------------------------- |
| webhook      | `GET /`             | Negotiate: returns the domain filter     |
| webhook      | `GET /records`      | Current endpoints                        |
| webhook      | `POST /records`     | Apply planned changes (`204` on success) |
| webhook      | `POST /adjustendpoints` | Provider-specific adjustment         |
| ops          | `GET /healthz`      | Liveness/readiness probe                 |
| ops          | `GET /metrics`      | Prometheus metrics                       |

Transient NetBird failures surface as `5xx` (ExternalDNS retries);
permanent failures surface as `4xx`.

## Build & test

```sh
go build ./...
go test ./...
golangci-lint run ./...
docker build -t external-dns-netbird:dev .
```

Image: `ghcr.io/lazygeniusman/home-ops/infra/external-dns-netbird` (`:dev`
on any branch push, `:latest` + version on `external-dns-netbird-v*` tags).
