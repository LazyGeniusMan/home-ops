# Fleet (platform team)

D2 fleet layer for the home-ops monorepo. Defines the clusters, the Flux
Operator lifecycle, tenant delivery (`tenants/`), and the Terraform bootstrap
(`terraform/`). Adapted from the upstream
[d2-fleet](https://github.com/controlplaneio-fluxcd/d2-fleet) reference
(clusters match the Talos clusters in `talos/clusters/`; components are
onboarded per-directory in `flux/apps/components/` and
`flux/infra/components/`).

## Layout

```text
flux/fleet/
├── clusters/
│   ├── acme-prd-bdo1-talos-apps-01/  # prd cluster (ENVIRONMENT=prd, syncs OCI tag stable)
│   │   ├── flux-system/ # FluxInstance + operator ResourceSet + values + runtime-info
│   │   └── tenants.yaml # tenants Kustomization (renders tenants/overlays/acme-prd-bdo1-talos-apps-01)
│   ├── acme-dev-bdo1-talos-apps-01/  # dev cluster (ENVIRONMENT=dev, syncs OCI tag dev)
│   │   ├── flux-system/
│   │   └── tenants.yaml # tenants Kustomization (renders tenants/overlays/acme-dev-bdo1-talos-apps-01)
│   └── update/          # image-automation cluster, NOT a Talos cluster
│                        # (ENVIRONMENT=dev, syncs OCI tag dev)
│       ├── flux-system/
│       └── automation.yaml  # ImageUpdateAutomation ResourceSet for infra + apps areas
├── tenants/
│   ├── policies.yaml    # source allowlist + ValidatingAdmissionPolicy
│   ├── infra.yaml       # ResourceSet: per-component namespace + OCIRepository + Kustomizations
│   ├── apps.yaml        # ResourceSet: per-component namespace + OCIRepository + Kustomizations
│   └── overlays/        # per-cluster selection (one dir per cluster; prd pass-through, dev skips win11-vm)
│       ├── README.md    # mechanism, onboarding, CLUSTER_NAME/CLUSTER_DOMAIN consumption
│       ├── acme-prd-bdo1-talos-apps-01/  # prd overlay (full set, no patches active)
│       └── acme-dev-bdo1-talos-apps-01/  # dev overlay (skips win11-vm via active patch)
└── terraform/           # OpenTofu bootstrap of the Flux Operator (no live apply in CI)
```

Envs are `dev` / `prd` (component overlays are `{base,dev,prd}/`).

## Bootstrap on barebone Talos

Talos ships barebone (no CNI, no CoreDNS, no kube-proxy — see
`talos/clusters/_base/patches.yml`), so `terraform/` runs a host-networked
bootstrap Job (`job.host_network = true`) that installs the Cilium chart
from the module's `prerequisites` slot **before** the Flux Operator; Cilium
+ CoreDNS then reconcile as infra tenants (`tenants/infra.yaml`, inputs
#1/#2), with Flux adopting the bootstrap-installed Cilium release. Only
Cilium is a prerequisite — CoreDNS follows via Flux once the Job's host
DNS (Talos `ResolverConfig` upstreams) has done the registry pulls. The
only per-cluster bootstrap difference is the Talos API VIP passed as
`var.cilium_k8s_service_host` (prd `.198`, dev `.248`). Details in
`terraform/README.md`.

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

## Upgrades

Fleet content promotes dev → prd through `ARTIFACT_TAG`, never by forking files:

1. **Propose.** The `update` cluster's `ImageUpdateAutomation` (30m) opens
   `image-updates-*` PRs for the infra + apps update policies
   (`ImageRepository` 12h); fleet's own OCI artifact carries no update policy —
   it versions by release tag.
2. **Merge (human).** Automation proposes, never merges — review the PR and
   merge by hand.
3. **Bake on dev.** Every `main` commit publishes `dev` (+ `dev-<sha>`); the
   dev and `update` clusters sync `dev`, so validate there first.
4. **Promote to prd.** Tag `flux-fleet-vX.Y.Z` to publish `stable`
   (+ `stable-<version>`) with cosign signatures; prd pins `stable` with
   cosign verification against the release workflow and tag.

Cadences: tenants `OCIRepository` 5m, tenant Kustomizations 30m, charts 1h,
`ImageUpdateAutomation` 30m, ResourceSets 5m, per-cluster `tenants`
Kustomization 12h. Per-cluster differences stay in
`tenants/overlays/` (selection patches only — overlays carry no version
pins), so promotion is always a tag move, never a file fork.
