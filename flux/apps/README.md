# Apps (tenant workloads)

D2 apps layer for the home-ops monorepo, adapted from upstream
[d2-apps](https://github.com/controlplaneio-fluxcd/d2-apps). Holds tenant
workloads delivered per-namespace with least-privilege RoleBindings. Base
only — real applications land in §§8-14.

## Layout

```text
flux/apps/
├── components/<name>/
│   ├── base/                  # Helm OCI source + HelmRelease
│   ├── prd/                   # kustomize patches over ../base
│   ├── stg/
│   └── dev/
└── update-policies/<name>.yaml  # ImageRepository + ImagePolicy per app
```

## Artifacts

`oci://ghcr.io/lazygeniusman/home-ops/apps/<component>`, tagged `dev`
(+ `dev-<sha>`, main commits touching the component dir) and `stable`
(+ `stable-<version>`, area releases tagged `flux-apps-v*`, which publish
every matrix component). The `acme-prd-bdo1-talos-apps-01` cluster consumes
`${ARTIFACT_TAG}` (`stable`) with cosign verification against the
release workflow subject.

## Onboarding (§§8-14)

1. Create `components/<name>/` following the placeholder skeleton, Helm OCI
   only (`OCIRepository` + `layerSelector`, `HelmRelease.chartRef` with
   drift detection). Telemetry stays off by default; enable monitoring per app.
2. Add the `tenant: <name>` input to `flux/fleet/tenants/apps.yaml`.
3. Add `<name>` to the components matrix in
   `.github/workflows/flux-apps-push.yaml`.
4. Add `update-policies/<name>.yaml` (ImageRepository + ImagePolicy).
