# Apps (tenant workloads)

Tenant workloads delivered per-namespace with least-privilege RoleBindings.
Current applications (8): clickstack, coder, flux-operator-ui, headlamp,
hubble-ui, matrix, talos-vm, win11-vm.

## Layout

```text
flux/apps/
├── components/<name>/
│   ├── base/                  # Helm OCI source + HelmRelease
│   ├── dev/                   # kustomize patches over ../base
│   └── prd/                   # kustomize patches over ../base
└── update-policies/<name>.yaml  # ImageRepository + ImagePolicy per app
```

## Artifacts

`oci://ghcr.io/lazygeniusman/home-ops/apps/<component>`, tagged `dev`
and `stable` (area releases `flux-apps-v*`). Prd consumes `${ARTIFACT_TAG}`
(`stable`) with cosign verification.

## Onboarding

1. Create `components/<name>/` with `base/`, `dev/`, `prd/` overlays
   (Helm OCI only: `OCIRepository` + `chartRef`).
2. Add the `tenant: <name>` input to `flux/fleet/tenants/apps.yaml`.
3. Add `<name>` to the components matrix in
   `.github/workflows/flux-apps-push.yaml`.
4. Add `update-policies/<name>.yaml` (ImageRepository + ImagePolicy).

## Upgrades

ImageUpdateAutomation (30m) proposes `image-updates-*` PRs against each
`$imagepolicy` marker; human merges, dev soaks first, area release
(`flux-apps-v*`) promotes to prd. Apps `dependsOn` infra Ready.
