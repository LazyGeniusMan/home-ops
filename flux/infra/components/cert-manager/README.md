# cert-manager (§9.3)

cert-manager v1.21.1, Cloudflare DNS-01 `ClusterIssuer/letsencrypt`, and the
`*.home-ops.yansyah.my.id` wildcard `Certificate` (`wildcard-home-ops-tls`).

## Credentials

`ExternalSecret/cloudflare-api-token` syncs the token from Proton Pass
(`pass://acme-prd-bdo1-talos-apps-01/cert-manager/cloudflare-api-token`,
token needs Zone:Read + DNS:Edit). Seed the vault entry with pass-cli.

## Environments

Base issuer carries no ACME server; `prd` sets LE production,
`stg` sets LE staging (patch on `ClusterIssuer/letsencrypt`).

## Gateway TLS (§8 coordination)

The `Certificate` lives in the `cert-manager` namespace; a Gateway listener
needs the Secret in the Gateway's namespace — copy `wildcard-home-ops-tls`
there or relocate this `Certificate` once the gateway namespace exists.

## Telemetry-off / monitoring / updates

- No reporting knobs in chart values; `prometheus.enabled` only adds scrape
  annotations. `ServiceMonitor` disabled until CRDs land.
- Renewal is automatic (2/3 lifetime). Chart bumps via
  `update-policies/cert-manager.yaml` → PR automation.
