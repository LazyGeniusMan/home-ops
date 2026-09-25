# tofu-controller

Tofu Controller v0.16.5 (chart 0.16.5,
`oci://ghcr.io/flux-iac/charts/tofu-controller`): Flux-native
Terraform/OpenTofu reconciler for `kind: Terraform` objects
(`terraforms.infra.contrib.fluxcd.io`). Chart == app (v0.16.5 for both
`ghcr.io/flux-iac/tofu-controller` and `ghcr.io/flux-iac/tf-runner`); on
chart bumps set `image.tag` and `runner.image.tag` to the new appVersion
together.

## Layout

`controllers/{base,dev,prd}` (OCIRepository + HelmRelease; dev/prd inherit
base with no patches) and `configs/{base,dev,prd}` (empty base -- no `kind:
Terraform` objects ship yet).

## Namespace / RBAC

The controller runs in its tenant namespace (`tofu-controller`), not
`flux-system`. The chart parameterises everything on `.Release.Namespace`;
the tenant auto-creates the Namespace + `flux` ServiceAccount +
cluster-admin binding (same as every other infra component). Runner reach:
each runner Pod serves gRPC on port 30000 to the controller; the controller
pulls source tarballs from source-controller and posts events to
notification-controller (both port 80).

## Runner namespaces

`runner.serviceAccount.allowedNamespaces` (base values):

- `flux-system` (chart default, kept),
- `zitadel`, `clickstack`, `hubble-ui`, `flux-operator-ui`, `headlamp`,
  `coder`, `seaweedfs`, `matrix`, `netbird` -- namespaces for `Terraform`
  CRs.

`watchAllNamespaces: true` (the default, stated explicitly) so the
controller watches Terraform CRs outside its own namespace. Rule: extend
`allowedNamespaces` whenever a `Terraform` CR lands in a namespace not on
this list -- runners only spawn where their ServiceAccount/token Secret
exists.

`allowCrossNamespaceRefs: true` opts in to cross-namespace `sourceRef`s
(upstream default `false` since 0.16.0): tenant-namespace `Terraform` CRs
point at the shared `OCIRepository/infra` source instead of per-consumer
git pins.

## Backend

Default in-cluster Kubernetes backend: tfstate stored as Secrets
(`tfstate-<workspace>-<secretSuffix>`) inside the cluster. Set an explicit
`.spec.backendConfig` only if a module ever needs to leave the cluster.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | `replicaCount: 1` (singleton; leader-elected) |
| `prd` | `replicaCount: 1` (singleton; raise to 2-3 only for multi-node HA) |

## Telemetry / monitoring / updates

No phone-home knobs in chart values. `metrics.enabled: false`, no
`ServiceMonitor` shipped. Branch Planner off (`branchPlanner.enabled:
false`). Bumps: `update-policies/tofu-controller.yaml` (>=0.16.5, marker
`infra:tofu-controller:tag`) -> PR automation (chart tag + both image tags
together).
Changelog: https://github.com/flux-iac/tofu-controller/releases.
