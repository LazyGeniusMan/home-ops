# NetBird

Reusable NetBird reverse-proxy Terraform root + its OCI delivery shell. The
`controllers/` and `configs/` dirs are empty shells -- this component exists
so the shared root in `terraform/` is published as
`oci://ghcr.io/lazygeniusman/home-ops/infra/netbird` and consumable via
cross-namespace `sourceRef` (same pattern as the zitadel `terraform/`
root). Live consumers: matrix `element-proxy` and zitadel `login-proxy`
Terraform CRs.

## Layout

```text
flux/infra/components/netbird/
├── terraform/           # the shared root (main/variables/outputs/versions +
│                        #   README) -- the only reconciled content, via
│                        #   consumer Terraform CRs (path: ./terraform)
├── controllers/{base,dev,prd}/  # empty (no HelmRelease ships)
├── configs/{base,dev,prd}/      # empty (no Terraform CR ships here)
└── examples/
    └── consumer-terraform.yaml  # copy-paste skeleton for app components
```

`controllers/` and `configs/` stay empty because Terraform CRs live in the
consuming app namespaces -- per-slice state Secrets stay namespace-local to
each app. The per-tenant `infra-configs`/`infra-controllers` Kustomizations
still require those paths in the OCI artifact, hence the shells.

## Consumers

Copy `examples/consumer-terraform.yaml` into the app's `base/` directory
(`<app>-terraform-vars` ExternalSecret + `<app>-proxy` Terraform CR), then
add per-env overlay patches for hosts/zone IDs (positional `/spec/vars/*`).
Full contract in `terraform/README.md`. The shared root takes no per-app
host var -- the service FQDN rides the consumer's `domain` var.

## Credentials

No chart-minted central secret exists, so no cross-namespace ESO mirror:
consumers read the NetBird PAT + Cloudflare API token straight from Proton
Pass through the cluster-scoped `proton-pass` ClusterSecretStore. Seed once
per app (never commit):

- `pass://<cluster>/<app>/netbird-pat` -- NetBird management PAT,
  reverse-proxy read/write
- `pass://<cluster>/<app>/cloudflare-api-token` -- Cloudflare API token,
  DNS edit on the zone

## Updates

No `update-policies/netbird.yaml`: nothing tracks a chart or image --
provider pins live in `terraform/versions.tf` (`netbirdio/netbird ~>
0.0.10`, `cloudflare/cloudflare ~> 5.0`); bump them there. Consumer CRs
land in app namespaces already on the runner `allowedNamespaces` list.
Changelogs: https://github.com/netbirdio/terraform-provider-netbird/releases,
https://github.com/cloudflare/terraform-provider-cloudflare/releases.

## Environments

| Env | Patches |
| --- | --- |
| `dev` | none -- inherits `../base` unchanged |
| `prd` | none -- inherits `../base` unchanged |
