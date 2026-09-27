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
vault ref, email, and wildcard `Certificate`. Duplicate-cert discipline:
cert-manager Secrets are namespace-local, so every namespace needing edge
TLS mints its own duplicate wildcard `Certificate` from this ClusterIssuer
(same issuer, same dnsNames) — gateway-api, coder (two certs for the nested
`*.coder` shape), matrix (two certs for the nested `*.matrix` shape), and
every other TLS namespace. One wildcard covers a single DNS label only.

Rate-limit math (why LE prod in dev too): dev mints ~10 wildcard Certificates
(once each, then auto-renewal at 2/3 lifetime = ~60d), far below the LE
certificates-per-domain (50/week) and duplicate-certificate (5/week) limits.
LE staging stays off — staging certs are untrusted and would break the dev
NetBird/OIDC trust chain; if dev ever exceeds ~20 certs, switch the dev
overlay server to `https://acme-staging-v02.api.letsencrypt.org/directory`.

## Telemetry / monitoring / updates

No reporting knobs in chart values (`prometheus.enabled` only adds scrape
annotations). `ServiceMonitor` on (monitoring CRDs via the infra-crds tenant).
Renewal automatic (2/3 lifetime). Chart
bumps: `update-policies/cert-manager.yaml` (>=1.21.2, marker
`infra:cert-manager:tag`) -> PR automation.
Changelog: https://github.com/cert-manager/cert-manager/releases.
