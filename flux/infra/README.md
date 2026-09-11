# Infra (platform components)

D2 infra layer for the home-ops monorepo, adapted from upstream
[d2-infra](https://github.com/controlplaneio-fluxcd/d2-infra). Holds cluster
add-ons (CRDs + controllers) reconciled by Flux as cluster admin. Current
components (17): cert-manager, cilium, clickhouse, cnpg, coredns, cosi,
dragonfly, external-dns, external-secrets, gateway-api, kubevirt,
local-path-provisioner, metrics-server, multus, seaweedfs, tofu-controller,
zitadel.

## Layout

```text
flux/infra/
├── components/<name>/
│   ├── controllers/{base,dev,stg,prd}/  # Helm OCI sources + HelmReleases
│   └── configs/{base,dev,stg,prd}/      # component configuration overlays
└── update-policies/<name>.yaml                 # ImageRepository + ImagePolicy per component
```

## Artifacts

`oci://ghcr.io/lazygeniusman/home-ops/infra/<component>`, tagged `dev`
(+ `dev-<sha>`, main commits touching the component dir) and `stable`
(+ `stable-<version>`, area releases tagged `flux-infra-v*`, which publish
every matrix component). The `acme-prd-bdo1-talos-apps-01` cluster consumes
`${ARTIFACT_TAG}` (`stable`) with cosign verification against the
release workflow subject.

## Onboarding

1. Create `components/<name>/` with `controllers/{base,dev,stg,prd}/` and
   `configs/{base,dev,stg,prd}/`, Helm OCI only (`OCIRepository` +
   `layerSelector`, `HelmRelease.chartRef`). Telemetry stays off by
   default; enable monitoring per component.
2. Add the `tenant: <name>` input to `flux/fleet/tenants/infra.yaml`.
3. Add `<name>` to the components matrix in
   `.github/workflows/flux-infra-push.yaml`.
4. Add `update-policies/<name>.yaml` (ImageRepository + ImagePolicy).
