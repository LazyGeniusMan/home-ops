# Fleet (platform team)

D2 fleet layer for the home-ops monorepo. Defines the clusters, the Flux
Operator lifecycle, tenant delivery (`tenants/`), and the Terraform bootstrap
(`terraform/`). Adapted from the upstream
[d2-fleet](https://github.com/controlplaneio-fluxcd/d2-fleet) reference
(single `home` cluster replaces the upstream staging/prod fleet; components
are onboarded per-directory in §§8-14).

## Layout

```text
flux/fleet/
├── clusters/
│   ├── home/            # production cluster (syncs OCI tag latest-stable)
│   │   ├── flux-system/ # FluxInstance + operator ResourceSet + values + runtime-info
│   │   └── tenants.yaml # tenants Kustomization (renders ../tenants with runtime substitution)
│   └── update/          # image-automation cluster (syncs OCI tag latest)
│       ├── flux-system/
│       └── automation.yaml  # ImageUpdateAutomation ResourceSet for infra + apps areas
├── tenants/
│   ├── policies.yaml    # source allowlist + ValidatingAdmissionPolicy
│   ├── infra.yaml       # ResourceSet: per-component namespace + OCIRepository + Kustomizations
│   └── apps.yaml        # ResourceSet: per-component namespace + OCIRepository + Kustomization
└── terraform/           # OpenTofu bootstrap of the Flux Operator (no live apply in CI)
```

## Artifacts

`oci://ghcr.io/lazygeniusman/home-ops/fleet`, tagged `latest` (main commits)
and `latest-stable` (`fleet-v*` release tags). The `home` cluster pins
`latest-stable` with cosign verification against the release workflow and tag;
the `update` cluster tracks `latest` mirrored from main.

## Onboarding a component

1. Create the component directory (`flux/infra/components/<name>` or
   `flux/apps/components/<name>`).
2. Add a matching `tenant` input in `tenants/infra.yaml` (or `apps.yaml`).
3. Add the component to the workflow matrix in
   `.github/workflows/flux-infra-push.yaml` (or `flux-apps-push.yaml`).
4. Create the update policy in the area's `update-policies/`.
