# Clickstack observability

HyperDX v2 (logs/traces/metrics UI) + OTel Collector on namespace-local ClickHouse (1 shard x 2 replicas) and FerretDB on CNPG.

## Layout

`base/` holds every manifest (`clickstack.yaml`, `ferretdb.yaml`, `oauth2-proxy.yaml`, ClickHouseInstallation, CNPG Cluster, wildcard certificate, HTTPRoute); `{dev,prd}/` patch hostnames, vault refs, and endpoints.

## Images

| Image | Pin |
|---|---|
| HyperDX app `docker.hyperdx.io/hyperdx/hyperdx` | `v2.7.1` (hdx-oss-v2 chart 0.8.4) |
| Collector `docker.hyperdx.io/hyperdx/hyperdx-otel-collector` | `v2.7.1` |
| FerretDB `ghcr.io/ferretdb/ferretdb` | `2.7.0` |
| oauth2-proxy chart / app | `10.7.0` / `v7.15.4` |

## Backends

- ClickHouse (`base/clickstack-clickhouse.yaml`): namespace-local CHI, 1 shard x 2 replicas, S3 backups to `clickstack/`. Keeper reuses the shared infra ensemble.
- FerretDB (`base/ferretdb-postgres.yaml` + `base/ferretdb.yaml`): CNPG Cluster (3 instances prd, 1 dev) + stateless Mongo-wire proxy singleton. `sslmode=require`.

| Item | Value |
|---|---|
| HyperDX → FerretDB | `mongodb://ferretdb.clickstack.svc:27017/hyperdx` |
| FerretDB → Postgres | `ferretdb-rw.clickstack.svc:5432/ferretdb` |
| HyperDX → ClickHouse | `http://clickstack-clickstack.clickstack.svc:8123` |
| Collector → ClickHouse | `tcp://clickstack-clickstack.clickstack.svc:9000?dial_timeout=10s` |

## Auth

Per-instance `oauth2-proxy` fronts the UI (`:4180` → `http://clickstack.clickstack.svc:3000`):

| Item | Value |
|---|---|
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Redirect | `https://clickstack.home-ops.yansyah.my.id/oauth2/callback` |
| Cookie | `_oauth2_proxy_clickstack`, domain `.home-ops.yansyah.my.id` |
| Gate | `allowed-group=clickstack-admin` |

## Credentials

`client-id` + `client-secret` from `clickstack-sso-outputs` (Terraform-owned, via `clickstack-k8s` store); cookie secret from `pass://acme-<env>-bdo1-talos-apps-01/clickstack/oauth2-proxy-cookie-secret`.

## Routing

HTTPRoute on shared `main` Gateway: `clickstack.home-ops.yansyah.my.id` → `oauth2-proxy:4180`. TLS at the Gateway via the in-namespace wildcard Certificate.

## Probes

Fixed-path exceptions (why-comments in `base/clickstack.yaml`): HyperDX app `/health` (`:8000`), collector `/` (`:13133`), FerretDB TCP `:27017`.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | app 1, collector 1, proxy 1, ferretdb 1, Cluster 1, CHI 1x1 | vault refs, hostnames, counts → 1 |
| `prd` | app 2, collector 2, proxy 1, ferretdb 1, Cluster 3, CHI 1x2 | vault refs, hostnames, production counts |

oauth2-proxy and FerretDB stay singletons (1) in every env.

## Updates

Policy `update-policies/clickstack.yaml` (`$imagepolicy` markers; proxy markers shared with hubble-ui + flux-operator-ui, bump together). Changelogs: [HyperDX](https://github.com/hyperdxio/hyperdx/releases) · [FerretDB](https://github.com/FerretDB/FerretDB/releases) · [proxy chart](https://github.com/oauth2-proxy/manifests/releases) · [proxy image](https://github.com/oauth2-proxy/oauth2-proxy/releases).
