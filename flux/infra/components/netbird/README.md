# NetBird

Reusable NetBird reverse-proxy Terraform root + its OCI delivery shell (`oci://ghcr.io/lazygeniusman/home-ops/infra/netbird`, consumable via cross-namespace `sourceRef`). The shared root in `terraform/` is the only reconciled content, via consumer Terraform CRs (path: `./terraform`). Live consumers: matrix `element-proxy` and zitadel `login-proxy`.

`controllers/` and `configs/` stay empty shells (Terraform CRs live in consuming app namespaces so per-slice state Secrets stay namespace-local); the per-tenant Kustomizations still require those paths in the OCI artifact.

## Consumers

Copy `examples/consumer-terraform.yaml` into the app's `base/` (`<app>-terraform-vars` ExternalSecret + `<app>-proxy` Terraform CR), then add per-env overlay patches for hosts/zone IDs. `service_lb_ip` is required (no default): every consumer passes the per-env Cilium LB VIP explicitly (dev 192.168.1.249, prd 192.168.1.199). `target_port` defaults to 3000 — override with the in-mesh backend listener port per consumer (element-web 80). Full contract in `terraform/README.md`.

## Credentials

No chart-minted central secret — consumers read the NetBird PAT + Cloudflare API token straight from Proton Pass through the cluster-scoped `proton-pass` ClusterSecretStore: `pass://<cluster>/<app>/netbird-pat` (reverse-proxy read/write), `pass://<cluster>/<app>/cloudflare-api-token` (DNS edit on the zone).

## Environments

| Env | Patches |
| --- | --- |
| `dev` | none — inherits `../base` unchanged |
| `prd` | none — inherits `../base` unchanged |

## Updates

No update policy (nothing tracks a chart or image) — provider pins live in `terraform/versions.tf` (`netbirdio/netbird ~> 0.0.10`, `cloudflare/cloudflare ~> 5.0`); bump them there. Changelogs: https://github.com/netbirdio/terraform-provider-netbird/releases, https://github.com/cloudflare/terraform-provider-cloudflare/releases.
