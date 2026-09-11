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
so every file under `overlays/` stays a plain valid Kubernetes/Kustomize
manifest (CI `validate.sh` kubeconforms each `*.yaml` directly AND builds
each `kustomization.yaml`). Example patches live as commented inline
`patch: |-` blocks; enabling one is an uncomment, never a new file.

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

Per-cluster differences are RFC 6902 JSON patches (`patches:` entries in the
overlay `kustomization.yaml`), applied uniformly to any of the three
ResourceSets. Why JSON patches and not strategic-merge `$patch: delete`?

- `ResourceSet` is a Flux custom resource: strategic-merge has no merge-key
  schema for `spec.inputs[]`, so SMP cannot address a single tenant entry.
- `spec.inputs[]` is an **ordered list**; JSON `remove` is index-based, so
  every list removal is guarded by a preceding `test` op on the tenant name
  at that index. If someone reorders `tenants/{apps,infra}.yaml`, the
  `kustomize build` **fails loudly** instead of silently dropping the wrong
  tenant.
- Removals go **highest index first** so earlier ops do not shift later
  targets. Re-check the 0-based index in `tenants/apps.yaml` (or
  `infra.yaml`) every time you write a patch; the current order is noted in
  the comments of each overlay `kustomization.yaml`.

Patches are inline `patch: |-` YAML blocks (not separate `path:` files and
not `*.json`) so the op list stays readable and every file remains a valid
manifest.

## Adding a per-cluster exception (onboarding)

1. Add an inline RFC 6902 `patches:` block in
   `tenants/overlays/<cluster>/kustomization.yaml` (keep `test` guards on
   list removals, highest index first).
2. Verify locally:
   `kustomize build flux/fleet/tenants/overlays/<cluster> --load-restrictor=LoadRestrictionsNone`
3. Open a PR; CI (`flux-fleet-validate`) auto-discovers the overlay
   `kustomization.yaml` and builds it.

## Adding a brand-new cluster

1. `mkdir tenants/overlays/<new-cluster>` with a `kustomization.yaml`
   copied from an existing overlay (pass-through resources only).
2. Point `clusters/<new-cluster>/tenants.yaml` at
   `path: ./tenants/overlays/<new-cluster>`.
3. Extend this README's tree + the fleet README layout.

## Verifying an overlay renders

To confirm an overlay renders the expected set:

```bash
kustomize build flux/fleet/tenants/overlays/<cluster> \
  --load-restrictor=LoadRestrictionsNone
```

(The `flux-system` OCIRepository `flux-system` built by the operator runs
with `--load-restrictor=LoadRestrictionsNone` semantics — stock kustomize
would reject the `../../` parent traversal, but the controller's embedded
kustomize build allows it, matching how `validate.sh` builds overlays.)

## CLUSTER_NAME / CLUSTER_DOMAIN consumption

`CLUSTER_NAME` / `CLUSTER_DOMAIN` (plus `ARTIFACT_TAG` / `ENVIRONMENT`) are
already plumbed to every tenant namespace: each ResourceSet copies
`flux-system/flux-runtime-info` into `<tenant>/flux-runtime-info` via
`copyFrom`, and every tenant Kustomization declares
`postBuild.substituteFrom` on that ConfigMap. Any component manifest can
therefore consume `${CLUSTER_NAME}` / `${CLUSTER_DOMAIN}` today (e.g.
Ingress hosts, external-dns hostnames, per-cluster labels) with **zero
fleet changes** — add the variable reference in the component's
`base/` or `{dev,stg,prd}/` overlay and the substitution happens at
reconcile time. No component currently does so, which is intentional:
`CLUSTER_NAME` has zero consumers by default so behavior is identical on
both clusters until an operator opts a component in.
