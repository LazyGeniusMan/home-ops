# Fleet (platform team)

D2 fleet layer for the home-ops monorepo. Defines the clusters, the Flux
Operator lifecycle, tenant delivery (`tenants/`), and the Terraform bootstrap
(`terraform/`). Adapted from the upstream
[d2-fleet](https://github.com/controlplaneio-fluxcd/d2-fleet) reference
(clusters match the Talos clusters in `talos/clusters/`; components are
onboarded per-directory in §§8-14).

## Layout

```text
flux/fleet/
├── clusters/
│   ├── acme-prd-bdo1-talos-apps-01/  # prd cluster (ENVIRONMENT=prd, syncs OCI tag stable)
│   │   ├── flux-system/ # FluxInstance + operator ResourceSet + values + runtime-info
│   │   └── tenants.yaml # tenants Kustomization (renders ../tenants with runtime substitution)
│   ├── acme-dev-bdo1-talos-apps-01/  # dev cluster (ENVIRONMENT=dev, syncs OCI tag dev)
│   │   ├── flux-system/
│   │   └── tenants.yaml
│   └── update/          # image-automation cluster, NOT a Talos cluster
│                        # (ENVIRONMENT=stg, syncs OCI tag dev)
│       ├── flux-system/
│       └── automation.yaml  # ImageUpdateAutomation ResourceSet for infra + apps areas
├── tenants/
│   ├── policies.yaml    # source allowlist + ValidatingAdmissionPolicy
│   ├── infra.yaml       # ResourceSet: per-component namespace + OCIRepository + Kustomizations
│   └── apps.yaml        # ResourceSet: per-component namespace + OCIRepository + Kustomizations
└── terraform/           # OpenTofu bootstrap of the Flux Operator (no live apply in CI)
```

Envs are `dev` / `stg` / `prd` (component overlays are `{base,dev,stg,prd}/`).

## Artifacts

`oci://ghcr.io/lazygeniusman/home-ops/fleet`, tagged `dev` (+ `dev-<sha>`,
main commits) and `stable` (+ `stable-<version>`, `flux-fleet-v*` release
tags). The `acme-prd-bdo1-talos-apps-01` cluster pins `stable` with cosign
verification against the release workflow and tag; the `update` automation
cluster tracks `dev` mirrored from main.

## Onboarding a component

1. Create the component directory (`flux/infra/components/<name>` or
   `flux/apps/components/<name>`).
2. Add a matching `tenant` input in `tenants/infra.yaml` (or `apps.yaml`).
3. Add the component to the workflow matrix in
   `.github/workflows/flux-infra-push.yaml` (or `flux-apps-push.yaml`).
4. Create the update policy in the area's `update-policies/`.
