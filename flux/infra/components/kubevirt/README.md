# kubevirt

KubeVirt v1.9.0: operator via the first-party `helm-kubevirt` OCI chart (`controllers/base/kubevirt-operator.yaml`: `OCIRepository` + `HelmRelease`, CRD `CreateReplace`) + `KubeVirt` CR (configs) enabling virt-operator/api/controller. `projects/helm-kubevirt` wraps the release asset at publish time (no YAML committed); `ref.tag` = Chart.yaml `version`.

## Talos prerequisites

Verified, no Talos config change required: bare metal provides `/dev/kvm`; Multus macvlan needs no bridge NIC; `local-path-provisioner` covers CDI scratch; shared storage is only for LiveMigration (single-node: not required).

## The CR at a glance

- `featureGates: [LiveMigration, NetworkBindingPlugins]` (Multus needs the latter); `useEmulation: false` (real KVM on bare metal).
- `smbios` TalosCloud identity block from the upstream Talos guide.
- `workloadUpdateStrategy: [LiveMigrate]` — with node-local `local-ssd-nvme` volumes there is nowhere to migrate to, so upgrade-time VMIs restart on this single node.
- Guest power state is owned by the VM manifests (`runStrategy: Manual`), not by this CR.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | control-plane 2 (operator default), virt pods per-node |
| `prd` | control-plane 2 (operator default), virt pods per-node |

Dev and prd track `../base` with no patches.

## Updates

Supported only N-1 -> N, never skip a minor; the operand locks to the operator version (no `spec.imageTag`), so the operator roll is the upgrade. Bump: daily check PR bumps `projects/helm-kubevirt` (Chart.yaml `version` + `appVersion`), then bump the wrapper `ref.tag` (no `$imagepolicy`, atomic hand-bump, human merges); never reorder the `infra-configs` `dependsOn` `infra-controllers` RBAC ordering. Deletion is CRs-first: delete the `KubeVirt` CR and wait for operands to drain before deleting the operator bundle (deleting the operator first strands the CR `Terminating` behind its finalizer).
