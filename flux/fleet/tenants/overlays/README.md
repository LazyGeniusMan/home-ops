# tenants/overlays — per-cluster selection

One directory per workload cluster, named exactly like the cluster:

```text
tenants/overlays/
├── README.md                          # this file
├── acme-dev-bdo1-talos-apps-01/       # ENVIRONMENT=dev, ARTIFACT_TAG=dev
│   └── kustomization.yaml             # active patch: skips win11-vm
├── acme-prd-bdo1-talos-apps-01/       # ENVIRONMENT=prd, ARTIFACT_TAG=stable
│   └── kustomization.yaml             # pass-through + commented policy example
└── update/                            # update-1, automation-only
    └── kustomization.yaml             # policies.yaml only (never apps/infra)
```

Each overlay holds a single `kustomization.yaml` — no sidecar patch files —
so every file under `overlays/` stays a valid manifest. Patches are
commented inline `patch: |-` blocks; enabling one is an uncomment.

## How it works

Each `clusters/<name>/tenants.yaml` syncs `path: ./tenants/overlays/<name>`.
Each overlay lists the three shared ResourceSets (`apps.yaml`, `infra.yaml`,
`policies.yaml`) as resources; with no `patches:` active it renders them
as-is. Dev removes the `win11-vm` input; prd is pass-through; `update` lists
`policies.yaml` only.

## Patch rules

Per-cluster differences are inline RFC 6902 JSON patches in the overlay
`kustomization.yaml`. List removals carry `test` guards on the tenant name,
highest index first. Verify with:
`kustomize build flux/fleet/tenants/overlays/<cluster> --load-restrictor=LoadRestrictionsNone`.

## Adding a brand-new cluster

1. `mkdir tenants/overlays/<new-cluster>` with a `kustomization.yaml`
   copied from an existing overlay (pass-through resources only).
2. Point `clusters/<new-cluster>/tenants.yaml` at
   `path: ./tenants/overlays/<new-cluster>`.
3. Extend this README's tree + the fleet README layout.

## Variable consumption

`ARTIFACT_TAG` selects the OCI tag each cluster syncs (`dev` on dev +
update, `stable` on prd) and therefore which cosign identity verifies it.
`CLUSTER_NAME` / `CLUSTER_DOMAIN` (plus `ARTIFACT_TAG` / `ENVIRONMENT`) reach
every tenant namespace via `copyFrom` of `flux-system/flux-runtime-info` +
`postBuild.substituteFrom` — any component manifest can reference
`${CLUSTER_NAME}` / `${CLUSTER_DOMAIN}`.
