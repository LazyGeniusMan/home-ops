# cert-manager

cert-manager v1.21.2 (`oci://quay.io/jetstack/charts/cert-manager`) with
Cloudflare DNS-01 `ClusterIssuer/letsencrypt` and the
`*.home-ops.yansyah.my.id` wildcard `Certificate` (`wildcard-home-ops-tls`).

## Credentials

`ExternalSecret/cloudflare-api-token` syncs from Proton Pass
(`pass://<cluster>/cert-manager/cloudflare-api-token`, Zone:Read +
DNS:Edit). Seed with pass-cli.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | controller + webhook + cainjector 1 | LE production ACME server, dev vault ref, ACME email `hostmaster@home-ops-dev.yansyah.my.id`, `Certificate/wildcard-home-ops-dev` |
| `prd` | controller + webhook + cainjector 2 | LE production ACME server, prd vault ref, ACME email, `Certificate/wildcard-home-ops` |

Base issuer leaves the ACME server unset; overlays set LE production plus
vault ref, email, and wildcard `Certificate`. The gateway-api component
mints its own duplicate wildcard `Certificate` in its own namespace (same
issuer, same dnsNames) because Gateway listeners need the TLS Secret
namespace-local.

## Telemetry / monitoring / updates

No reporting knobs in chart values (`prometheus.enabled` only adds scrape
annotations). `ServiceMonitor` off. Renewal automatic (2/3 lifetime). Chart
bumps: `update-policies/cert-manager.yaml` (>=1.21.2, marker
`infra:cert-manager:tag`) -> PR automation.
Changelog: https://github.com/cert-manager/cert-manager/releases.
