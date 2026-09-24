# Apps (tenant workloads)

D2 apps layer for the home-ops monorepo, adapted from upstream
[d2-apps](https://github.com/controlplaneio-fluxcd/d2-apps). Holds tenant
workloads delivered per-namespace with least-privilege RoleBindings. Current
applications (8): clickstack, coder, flux-operator-ui, headlamp, hubble-ui,
matrix, talos-vm, win11-vm.

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
(+ `dev-<sha>`, main commits touching the component dir) and `stable`
(+ bare `<version>`, area releases tagged `flux-apps-v*`, which publish
every component in the release matrix). The `acme-prd-bdo1-talos-apps-01`
cluster consumes `${ARTIFACT_TAG}` (`stable`) with cosign verification
against the release workflow subject.

## Onboarding

1. Create `components/<name>/` with `base/`, `dev/`, `prd/`
   overlays, Helm OCI only (`OCIRepository` + `layerSelector`,
   `HelmRelease.chartRef`). Telemetry stays off by
   default; enable monitoring per app.
2. Add the `tenant: <name>` input to `flux/fleet/tenants/apps.yaml`.
3. Add `<name>` to the components matrix in
   `.github/workflows/flux-apps-push.yaml`.
4. Add `update-policies/<name>.yaml` (ImageRepository + ImagePolicy).

## Upgrades

Same 4-step loop as infra, one Kustomization per app:

1. **Propose.** The `update` cluster's `ImageUpdateAutomation` (30m) watches
   each `update-policies/<name>.yaml` `ImageRepository` (12h poll) and opens
   an `image-updates-*` PR against the `$imagepolicy` marker.
2. **Merge (human).** Automation proposes, never merges — review the PR and
   merge by hand.
3. **Bake on dev.** Merging to `main` publishes `dev` (+ `dev-<sha>`); the dev
   cluster syncs `dev`, so validate there first.
4. **Promote to prd.** Tag an area release (`flux-apps-v*`), which publishes
   `stable` (+ bare `<version>`); prd consumes `${ARTIFACT_TAG}` (`stable`)
   with cosign verification.

Each app ships as a single Kustomization (ResourceSets reconcile 5m; tenants
`OCIRepository` 5m, tenant Kustomizations 30m, charts 1h) with `dependsOn`
on infra (`infra-configs` Ready), so app upgrades
never run ahead of the platform. Overlays carry no pins — dev/prd differences
are kustomize patches only, so the same promotion rule covers both.
