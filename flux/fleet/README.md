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

`terraform/` runs a host-networked bootstrap Job (`job.host_network = true`)
that installs the Cilium chart from the module's `prerequisites` slot before
the Flux Operator; Cilium + CoreDNS then reconcile as infra tenants
(`tenants/infra.yaml`), with Flux adopting the bootstrap-installed Cilium
release. Only Cilium is a prerequisite — CoreDNS follows via Flux. The only
per-cluster difference is `var.cilium_k8s_service_host` (Talos API VIP:
prd `.198`, dev `.248`). Details in `terraform/README.md`.

## Artifacts

`oci://ghcr.io/lazygeniusman/home-ops/fleet`: `dev` (+ `dev-<sha>`, main
commits) and `stable` (+ bare `<version>`, `flux-fleet-v*` tags). Prd pins
`stable` with cosign verification; the `update` cluster tracks `dev`.

## Onboarding a component

1. Create the component directory (`flux/infra/components/<name>` or
   `flux/apps/components/<name>`).
2. Add a matching `tenant` input in `tenants/infra.yaml` (or `apps.yaml`).
3. Add the component to the workflow matrix in
   `.github/workflows/flux-infra-push.yaml` (or `flux-apps-push.yaml`).
4. Create the update policy in the area's `update-policies/`.

## Upgrades

Fleet promotes dev → prd through `ARTIFACT_TAG`:

1. **Propose.** The `update` cluster's `ImageUpdateAutomation` (30m) opens
   `image-updates-*` PRs (`ImageRepository` 12h); the fleet artifact itself
   versions by release tag, no update policy. Automation pushes through
   `flux-system/github-auth` (Terraform-seeded via `var.github_token` on the
   update bootstrap); the policies ResourceSet (`tenants/overlays/update`)
   gates it via `dependsOn`.
2. **Merge (human).** Automation proposes, never merges.
3. **Bake on dev.** Every `main` commit publishes `dev` (+ `dev-<sha>`);
   dev and `update` sync `dev` — validate there first.
4. **Promote to prd.** Tag `flux-fleet-vX.Y.Z` (cosigned `stable` + bare
   `<version>`); prd pins `stable` with cosign verification.

Cadences: FluxInstance OCIRepository 10m (semver `*`), tenant
`OCIRepository` 5m, tenant Kustomizations 30m, charts 1h,
`ImageUpdateAutomation` 30m, ResourceSets 5m, per-cluster `tenants`
Kustomization 30m. Per-cluster differences stay in
`tenants/overlays/` (selection patches only, no version pins).
