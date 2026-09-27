# Infra (platform components)

Cluster add-ons (CRDs + controllers) reconciled by Flux as cluster admin.
23 components: apprise-go-api, cert-manager, cilium, clickhouse, cnpg,
coredns, cosi, dragonfly, external-dns, external-secrets, gateway-api,
kubevirt, local-path-provisioner, metrics-server, multus, netbird,
otel-collectors, otel-operator, reloader, seaweedfs, tofu-controller, vpa,
zitadel.

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
(main commits) and `stable` (`flux-infra-v*` releases). Prd consumes
`${ARTIFACT_TAG}` (`stable`) with cosign verification.

## Pins

Infra chart refs pin by tag + `$imagepolicy` marker (no digests); the tag
floor lives in `update-policies/<name>.yaml`. Charts without a published
cosign identity carry an explicit `# No verify: <reason>` comment. App
workload images pin `tag@digest` instead. See AGENTS.md for the update
policy + upgrade lifecycle.

## Onboarding

1. Create `components/<name>/` with `controllers/{base,dev,prd}/` and
   `configs/{base,dev,prd}/`, Helm OCI only (`OCIRepository` +
   `layerSelector`, `HelmRelease.chartRef`). Telemetry off by default.
2. Add the `tenant: <name>` input to `flux/fleet/tenants/infra.yaml`.
3. Add `<name>` to the components matrix in
   `.github/workflows/flux-infra-push.yaml`.
4. Add `update-policies/<name>.yaml` (ImageRepository + ImagePolicy).
   Exception: `netbird` ships no chart/image (shared Terraform root only;
   provider pins live in its `terraform/versions.tf`), so it has no update
   policy — see `components/netbird/README.md`.

## Upgrades

Dev bakes `dev` first; promote to prd by tagging an area release
(`flux-infra-v*`), which publishes `stable`. Automation proposes, human
merges — see AGENTS.md.

Reconcile order inside each tenant is `infra-crds` → `infra-controllers` →
`infra-configs` (`infra-crds` is `prune: false`).
