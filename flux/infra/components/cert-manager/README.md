# cert-manager (§9.3)

cert-manager v1.21.1, Cloudflare DNS-01 `ClusterIssuer/letsencrypt`, and the
`*.home-ops.yansyah.my.id` wildcard `Certificate` (`wildcard-home-ops-tls`).

## Credentials

`ExternalSecret/cloudflare-api-token` syncs the token from Proton Pass
(`pass://acme-prd-bdo1-talos-apps-01/cert-manager/cloudflare-api-token`,
token needs Zone:Read + DNS:Edit). Seed the vault entry with pass-cli.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (controller + webhook + cainjector, pinned in `controllers/dev`) | LE production ACME server, vault `pass://acme-dev-bdo1-talos-apps-01/...`, ACME email `hostmaster@home-ops-dev.yansyah.my.id`, `Certificate/wildcard-home-ops-dev` |
| `prd` | 2 (controller + webhook + cainjector, pinned in `controllers/prd`) | LE production ACME server, vault `pass://acme-prd-bdo1-talos-apps-01/...`, ACME email, `Certificate/wildcard-home-ops` |

Base issuer carries no ACME server; `dev` and `prd` each set the LE
production server (patch on `ClusterIssuer/letsencrypt`, with per-env
vault ref, ACME email, and wildcard `Certificate`). Controllers inherit
`../base` replica placeholders — `controllers/dev` pins
controller/webhook/cainjector `replicaCount` to 1, `controllers/prd` pins
all three to 2.

Upstream reference (read-only): `/tmp/home-ops-docs/cert-manager-docs`.

## Gateway TLS (§8 coordination)

Gateway listeners need their TLS Secret in the Gateway's own namespace, so
the gateway-api component mints its own duplicate wildcard `Certificate`
there (same `ClusterIssuer/letsencrypt`, same dnsNames) instead of consuming
this namespace's `wildcard-home-ops-tls` Secret cross-namespace.

## Telemetry-off / monitoring / updates

- No reporting knobs in chart values; `prometheus.enabled` only adds scrape
  annotations. `ServiceMonitor` disabled until CRDs land.
- Renewal is automatic (2/3 lifetime). Chart bumps via
  `update-policies/cert-manager.yaml` → PR automation.

## Upgrade runbook

- Version source: the `OCIRepository` tag in
  `controllers/base/cert-manager.yaml` (chart v1.21.1).
- Changelog: https://github.com/cert-manager/cert-manager/releases.
- Bump: let the ImagePolicy PR land (marker `infra:cert-manager:tag`,
  `update-policies/cert-manager.yaml`).
- Verify: controller + webhook + cainjector Deployments `Ready`, then
  force-renew a test `Certificate` (`cmctl renew`) and check the
  wildcard Secrets re-issue.
