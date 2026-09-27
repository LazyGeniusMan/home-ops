# tofu-controller

Tofu Controller v0.16.5 (chart 0.16.5, `oci://ghcr.io/flux-iac/charts/tofu-controller`): Flux-native Terraform/OpenTofu reconciler for `kind: Terraform` objects (`terraforms.infra.contrib.fluxcd.io`). Chart == app — on chart bumps set `image.tag` and `runner.image.tag` to the new appVersion together.

## Layout

`controllers/{base,dev,prd}` (OCIRepository + HelmRelease; dev/prd inherit base with no patches) and `configs/{base,dev,prd}` (empty base — no `kind: Terraform` objects ship yet).

## Namespace / RBAC

The controller runs in its tenant namespace (`tofu-controller`), not `flux-system`; the tenant auto-creates the Namespace + `flux` ServiceAccount + cluster-admin binding. `watchAllNamespaces: true` so the controller watches Terraform CRs outside its own namespace. `allowCrossNamespaceRefs: true` opts in to cross-namespace `sourceRef`s (tenant `Terraform` CRs point at the shared `OCIRepository/infra` source instead of per-consumer git pins).

## Runner namespaces

`runner.serviceAccount.allowedNamespaces` (base values): `flux-system` (chart default, kept) + `zitadel`, `clickstack`, `hubble-ui`, `flux-operator-ui`, `headlamp`, `coder`, `seaweedfs`, `matrix`, `netbird`. Rule: extend `allowedNamespaces` whenever a `Terraform` CR lands in a namespace not on this list — runners only spawn where their ServiceAccount/token Secret exists.

## Backend

Default in-cluster Kubernetes backend: tfstate stored as Secrets (`tfstate-<workspace>-<secretSuffix>`) inside the cluster.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | `replicaCount: 1` (singleton; leader-elected) |
| `prd` | `replicaCount: 1` (singleton; raise to 2-3 only for multi-node HA) |

## Updates

`update-policies/tofu-controller.yaml` (>=0.16.5, marker `infra:tofu-controller:tag` + controller marker `infra:tofu-controller-app:tag` + runner marker `infra:tofu-runner:tag`; chart tag + both image tags together). Changelog: https://github.com/flux-iac/tofu-controller/releases.
