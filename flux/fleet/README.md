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
│   │   └── tenants.yaml # tenants Kustomization (renders tenants/overlays/acme-prd-bdo1-talos-apps-01)
│   ├── acme-dev-bdo1-talos-apps-01/  # dev cluster (ENVIRONMENT=dev, syncs OCI tag dev)
│   │   ├── flux-system/
│   │   └── tenants.yaml # tenants Kustomization (renders tenants/overlays/acme-dev-bdo1-talos-apps-01)
│   └── update/          # image-automation cluster, NOT a Talos cluster
│                        # (ENVIRONMENT=stg, syncs OCI tag dev)
│       ├── flux-system/
│       └── automation.yaml  # ImageUpdateAutomation ResourceSet for infra + apps areas
├── tenants/
│   ├── policies.yaml    # source allowlist + ValidatingAdmissionPolicy
│   ├── infra.yaml       # ResourceSet: per-component namespace + OCIRepository + Kustomizations
│   ├── apps.yaml        # ResourceSet: per-component namespace + OCIRepository + Kustomizations
│   └── overlays/        # per-cluster selection (one dir per cluster, pass-through by default)
│       ├── README.md    # mechanism, onboarding, CLUSTER_NAME/CLUSTER_DOMAIN consumption
│       ├── acme-prd-bdo1-talos-apps-01/  # prd overlay (full set by default)
│       └── acme-dev-bdo1-talos-apps-01/  # dev overlay (full set by default)
└── terraform/           # OpenTofu bootstrap of the Flux Operator (no live apply in CI)
```

Envs are `dev` / `stg` / `prd` (component overlays are `{base,dev,stg,prd}/`).

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
