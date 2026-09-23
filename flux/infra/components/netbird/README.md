# NetBird

Reusable NetBird reverse-proxy Terraform root + its OCI delivery shell. The
`controllers/` and `configs/` dirs are intentionally empty shells (see Layout)
— this component exists so the shared root in `terraform/` is published as
`oci://ghcr.io/lazygeniusman/home-ops/infra/netbird` and consumable via
cross-namespace `sourceRef` (same pattern as the zitadel component's
`terraform/` root). Live consumers: matrix `element-proxy` and zitadel
`login-proxy` Terraform CRs.

## Layout

```text
flux/infra/components/netbird/
├── README.md            # this file
├── terraform/           # the shared root (main/variables/outputs/versions +
│                        #   README) — the only reconciled content, via
│                        #   consumer Terraform CRs (path: ./terraform)
├── controllers/{base,dev,prd}/  # empty on purpose (no HelmRelease ships)
├── configs/{base,dev,prd}/      # empty on purpose (no Terraform CR ships here)
└── examples/
    └── consumer-terraform.yaml  # copy-paste skeleton for app components
```

`controllers/` and `configs/` stay empty (`resources: []`, inheriting-base
env overlays) because Terraform CRs live in the *consuming* app namespaces
— not beside the shared root — so per-slice state Secrets stay namespace-local
to each app. The per-tenant `infra-configs`/`infra-controllers` Kustomizations
still require those paths to exist in the OCI artifact, hence the shells.

## Consumers

Copy `examples/consumer-terraform.yaml` into the app's `base/` directory
(`<app>-terraform-vars` ExternalSecret + `<app>-proxy` Terraform CR), then
add the per-env overlay patches for hosts/zone IDs (positional
`/spec/vars/*`, same discipline as the zitadel consumers). Full contract in
`terraform/README.md`. The shared root takes no per-app host var — pass the
service FQDN via the consumer's `domain` var (see `element-proxy.yaml`).

## Credentials

No chart-minted central secret exists (unlike zitadel's FirstInstance
handoff), so there is no cross-namespace ESO mirror and no `*-handoff-rbac.yaml`:
consumers read the NetBird PAT + Cloudflare API token straight from Proton
Pass through the existing cluster-scoped `proton-pass` ClusterSecretStore.
Seed once per app with pass-cli (never commit):

- `pass://<cluster>/<app>/netbird-pat` — NetBird management PAT with
  reverse-proxy read/write
- `pass://<cluster>/<app>/cloudflare-api-token` — Cloudflare API token with
  DNS edit on the zone

## Updates

No `update-policies/netbird.yaml`: nothing here tracks a chart or image —
provider pins live in `terraform/versions.tf`
(`netbirdio/netbird ~> 0.0.10`, `cloudflare/cloudflare ~> 5.0`); bump them
there. No tofu-controller change either: consumer CRs land in app namespaces
already on the runner `allowedNamespaces` list.

## Environments

| Env | Patches |
| --- | ------- |
| `dev` | none — inherits `../base` unchanged |
| `prd` | none — inherits `../base` unchanged |

Upstream references (read-only): `/tmp/home-ops-docs/netbird-docs/src/pages/manage/reverse-proxy/`
(custom-domains, service-configuration), `/tmp/home-ops-docs/netbird-terraform-provider-docs`
(`reverse_proxy_domain`, `reverse_proxy_service`, `reverse_proxy_clusters`),
`/tmp/home-ops-docs/cloudflare-terraform-provider-docs` (`cloudflare_dns_record`).
