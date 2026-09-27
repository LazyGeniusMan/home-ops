# cert-manager

cert-manager v1.21.2 (`oci://quay.io/jetstack/charts/cert-manager`) with Cloudflare DNS-01 `ClusterIssuer/letsencrypt` and the `*.home-ops.yansyah.my.id` wildcard `Certificate` (`wildcard-home-ops-tls`).

## Credentials

`ExternalSecret/cloudflare-api-token` syncs `pass://<cluster>/cert-manager/cloudflare-api-token` (Zone:Read + DNS:Edit).

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | controller + webhook + cainjector 1 | LE production server, dev vault ref, ACME email `hostmaster@home-ops-dev.yansyah.my.id`, `Certificate/wildcard-home-ops-dev` |
| `prd` | controller + webhook + cainjector 2 | LE production server, prd vault ref, ACME email, `Certificate/wildcard-home-ops` |

Base issuer leaves the ACME server unset; overlays set LE production. Cert-manager Secrets are namespace-local, so every namespace needing edge TLS mints its own duplicate wildcard `Certificate` from this ClusterIssuer (one wildcard covers a single DNS label only). Dev uses LE production — staging certs are untrusted and would break the NetBird/OIDC trust chain; renewal automatic at 2/3 lifetime.

## Updates

`update-policies/cert-manager.yaml` (>=1.21.2, marker `infra:cert-manager:tag`). Changelog: https://github.com/cert-manager/cert-manager/releases.
