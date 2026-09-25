# tenants/overlays — per-cluster selection

One directory per workload cluster, named exactly like the cluster:

```text
tenants/overlays/
├── README.md                          # this file
├── acme-dev-bdo1-talos-apps-01/       # ENVIRONMENT=dev, ARTIFACT_TAG=dev
│   └── kustomization.yaml             # active patch: skips win11-vm
└── acme-prd-bdo1-talos-apps-01/       # ENVIRONMENT=prd, ARTIFACT_TAG=stable
    └── kustomization.yaml             # pass-through + commented policy example
```

Each overlay holds a single `kustomization.yaml` — no sidecar patch files —
so every file under `overlays/` stays a valid manifest. Patches are
commented inline `patch: |-` blocks; enabling one is an uncomment.

## How it works

Each `clusters/<name>/tenants.yaml` (a Flux Kustomization) syncs
`path: ./tenants/overlays/<name>`. Each overlay kustomization lists the
three shared ResourceSets as resources:

```yaml
resources:
  - ../../apps.yaml
  - ../../infra.yaml
  - ../../policies.yaml
```

With no `patches:` active, the overlay renders the shared set as-is.
The dev overlay has one active patch (removes the `win11-vm` input from
the `apps` ResourceSet); the prd overlay has no active patches.

## One mechanism for apps + infra + policies

Per-cluster differences are inline RFC 6902 JSON patches (`patches:` entries in
the overlay `kustomization.yaml`), applied uniformly to any of the three
ResourceSets; list removals carry `test` guards on the tenant name, highest
index first.

## Adding a per-cluster exception (onboarding)

1. Add an inline RFC 6902 `patches:` block in
   `tenants/overlays/<cluster>/kustomization.yaml` (keep `test` guards on
   list removals, highest index first).
2. Verify locally:
   `kustomize build flux/fleet/tenants/overlays/<cluster> --load-restrictor=LoadRestrictionsNone`
   (the controller allows the `../../` parent traversal, as does
   `validate.sh`).

## Adding a brand-new cluster

1. `mkdir tenants/overlays/<new-cluster>` with a `kustomization.yaml`
   copied from an existing overlay (pass-through resources only).
2. Point `clusters/<new-cluster>/tenants.yaml` at
   `path: ./tenants/overlays/<new-cluster>`.
3. Extend this README's tree + the fleet README layout.

## CLUSTER_NAME / CLUSTER_DOMAIN consumption

`CLUSTER_NAME` / `CLUSTER_DOMAIN` (plus `ARTIFACT_TAG` / `ENVIRONMENT`) are
plumbed to every tenant namespace: each ResourceSet copies
`flux-system/flux-runtime-info` into `<tenant>/flux-runtime-info` via
`copyFrom`, and every tenant Kustomization declares
`postBuild.substituteFrom` on that ConfigMap. Any component manifest can
consume `${CLUSTER_NAME}` / `${CLUSTER_DOMAIN}` (e.g. HTTPRoute hostnames,
per-cluster labels) by referencing the variable in the component's
`base/` or `{dev,prd}/` overlay. The `__CLUSTER_NAME__` placeholder is
consumed (rclone backup destinations, element-proxy `network_name`),
replaced by per-env overlay patches.
