# Fleet (platform team)

Day-2 fleet layer: clusters, Flux Operator lifecycle, tenant delivery
(`tenants/`), Terraform bootstrap (`terraform/`). Clusters match Talos
(`talos/clusters/`); components onboard per-directory in
`flux/apps/components/` and `flux/infra/components/`.

## Layout

```text
flux/fleet/
├── clusters/
│   ├── acme-prd-bdo1-talos-apps-01/  # prd (ENVIRONMENT=prd, ARTIFACT_TAG=stable)
│   ├── acme-dev-bdo1-talos-apps-01/  # dev (ENVIRONMENT=dev, ARTIFACT_TAG=dev)
│   │   # each: flux-system/ (FluxInstance, operator ResourceSet, values, runtime-info) + tenants.yaml (→ tenants/overlays/<name>)
│   └── update/  # image-automation cluster, NOT a Talos cluster (ENVIRONMENT=dev, ARTIFACT_TAG=dev)
│       # flux-system/ + automation.yaml (ImageUpdateAutomation ResourceSet
│       # for infra + apps, dependsOn policies Ready) + tenants.yaml
│       # (→ tenants/overlays/update, policies only)
├── tenants/
│   ├── policies.yaml    # source allowlist + ValidatingAdmissionPolicy
│   ├── infra.yaml       # ResourceSet: per-component namespace + OCIRepository + Kustomizations
│   ├── apps.yaml        # ResourceSet: per-component namespace + OCIRepository + Kustomizations
│   └── overlays/        # per-cluster selection (prd pass-through, dev skips win11-vm, update policies-only)
└── terraform/           # OpenTofu bootstrap of the Flux Operator (no live apply in CI)
```

Envs are `dev` / `prd` (component overlays `{base,dev,prd}/`).

## Bootstrap on barebone Talos

`terraform/` runs a host-networked bootstrap Job that installs Cilium from
the module's `prerequisites` slot before the Flux Operator; Cilium + CoreDNS
then reconcile as infra tenants with Flux adopting the Cilium release. Only
per-cluster difference is `var.cilium_k8s_service_host` (Talos API VIP: prd
`.198`, dev `.248`; not the LB pool VIPs `.199`/`.249`). See
`terraform/README.md`.

## Artifacts

`oci://ghcr.io/lazygeniusman/home-ops/fleet`: `dev` (main commits) and
`stable` (`flux-fleet-v*` tags). Prd pins `stable`; dev and the `update`
cluster track `dev`. Same rule covers the per-component `infra/<tenant>` +
`apps/<tenant>` artifacts. Cosign: dev verifies against the push-workflow
identity, prd/stable against the release identity.

## Onboarding a component

1. Create the component directory (`flux/infra/components/<name>` or
   `flux/apps/components/<name>`).
2. Add a matching `tenant` input in `tenants/infra.yaml` (or `apps.yaml`).
3. Add the component to the workflow matrix in
   `.github/workflows/flux-infra-push.yaml` (or `flux-apps-push.yaml`).
4. Create the update policy in the area's `update-policies/`.
5. If the component's chart lives under a new registry org, add its
   `oci://` prefix to the allowlist in `tenants/policies.yaml`.

## Upgrades

Fleet promotes dev → prd through `ARTIFACT_TAG`: bake `dev` on every `main`
commit, then tag `flux-fleet-vX.Y.Z` to publish cosigned `stable` for prd.
Automation proposes, human merges — see AGENTS.md. Per-cluster differences
stay in `tenants/overlays/` (selection patches only, no version pins).
