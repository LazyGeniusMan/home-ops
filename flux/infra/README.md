# Infra (platform components)

D2 infra layer for the home-ops monorepo, adapted from upstream
[d2-infra](https://github.com/controlplaneio-fluxcd/d2-infra). Holds cluster
add-ons (CRDs + controllers) reconciled by Flux as cluster admin. Current
components (20): apprise-go-api, cert-manager, cilium, clickhouse, cnpg,
coredns, cosi, dragonfly, external-dns, external-secrets, gateway-api,
kubevirt, local-path-provisioner, metrics-server, multus, netbird,
seaweedfs, tofu-controller, vpa, zitadel.

## Layout

```text
flux/infra/
├── components/<name>/
│   ├── controllers/{base,dev,prd}/  # Helm OCI sources + HelmReleases
│   └── configs/{base,dev,prd}/      # component configuration overlays
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

1. Create `components/<name>/` with `controllers/{base,dev,prd}/` and
   `configs/{base,dev,prd}/`, Helm OCI only (`OCIRepository` +
   `layerSelector`, `HelmRelease.chartRef`). Telemetry stays off by
   default; enable monitoring per component.
2. Add the `tenant: <name>` input to `flux/fleet/tenants/infra.yaml`.
3. Add `<name>` to the components matrix in
   `.github/workflows/flux-infra-push.yaml`.
4. Add `update-policies/<name>.yaml` (ImageRepository + ImagePolicy).
   Exception: `netbird` ships no chart/image (shared Terraform root only;
   provider pins live in its `terraform/versions.tf`), so it intentionally
   has no update policy — see `components/netbird/README.md`.

## Upgrades

Chart and image bumps flow through automation; humans merge, Flux promotes:

1. **Propose.** The `update` cluster's `ImageUpdateAutomation` (30m) watches
   each `update-policies/<name>.yaml` `ImageRepository` (12h poll) and opens
   an `image-updates-*` PR against the `$imagepolicy` marker. The operator's
   own `OCIRepository` floats semver `*`.
2. **Merge (human).** Automation proposes, never merges — review the PR and
   merge by hand.
3. **Bake on dev.** Merging to `main` publishes `dev` (+ `dev-<sha>`); the dev
   cluster syncs `dev`, so validate there first.
4. **Promote to prd.** Tag an area release (`flux-infra-v*`), which publishes
   `stable` (+ `stable-<version>`); prd consumes `${ARTIFACT_TAG}` (`stable`)
   with cosign verification.

Reconcile order inside each tenant is `infra-crds` → `infra-controllers` →
`infra-configs` (ResourceSets reconcile 5m; tenants `OCIRepository` 5m,
tenant Kustomizations 30m, charts 1h).
`infra-crds` is `prune: false` so a removed CRD file never cascade-deletes
CRs; `infra-controllers` and `infra-configs` are `prune: true`, each gated on
the previous stage via `dependsOn`. `netbird` has no chart/image, so its
upgrades are provider-pin bumps in `terraform/versions.tf`, not
update-policy PRs.
