# kubevirt

KubeVirt v1.9.0: operator via the first-party `helm-kubevirt` OCI chart
(`controllers/base/kubevirt-operator.yaml`: `OCIRepository` + `HelmRelease`,
CRD `CreateReplace`) + `KubeVirt` CR (configs) enabling
virt-operator/api/controller.

## Talos prerequisites

Verified, no Talos config change required: bare metal provides `/dev/kvm`
(no KVM schematic extension exists or is needed); Multus macvlan needs no
bridge NIC; `local-path-provisioner` covers CDI scratch; shared storage is
only for LiveMigration (single-node: not required). Day-0 check:
`ls /dev/kvm` before first VM start.

## Source: first-party chart, not vendored

No official upstream chart — `projects/helm-kubevirt` wraps the release
asset (`ci/fetch.sh` stages
`https://github.com/kubevirt/kubevirt/releases/download/v1.9.0/kubevirt-operator.yaml`
at publish time; no YAML committed in the chart). Flux consumes the published
OCI artifact (`oci://ghcr.io/lazygeniusman/home-ops/projects/helm-kubevirt`,
`ref.tag` = Chart.yaml `version`).

- CR:
  `https://github.com/kubevirt/kubevirt/releases/download/v1.9.0/kubevirt-cr.yaml`
  (upstream CR is a near-empty skeleton; ours extends it per the Talos
  guide — see header in `configs/base/kubevirt-cr.yaml`).

## The CR at a glance

- `featureGates: [LiveMigration, NetworkBindingPlugins]` (Multus needs the
  latter); `useEmulation: false` (real KVM on bare metal).
- `smbios` TalosCloud identity block from the upstream Talos guide.
- `workloadUpdateStrategy: [LiveMigrate]`.
- Storage: default-class `local-ssd-nvme` used implicitly (the v1.9.0 CRD
  schema has no top-level storage/scratch knob).
- Metrics on where safe (prometheus annotations only, no ServiceMonitor);
  no phone-home flags upstream.
- `monitorNamespace` / `monitorAccount` / `serviceMonitorNamespace` unset:
  their defaults (`openshift-monitor` / `prometheus-k8s`) do not exist here.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | control-plane 2 (operator default), virt pods per-node |
| `prd` | control-plane 2 (operator default), virt pods per-node |

Dev and prd track `../base` with no patches. Virt-handler/virt-controller
scale with the cluster, not with a replica count here.

## runStrategy ownership

Guest power state is owned by the VM manifests (`runStrategy: Manual`), not
by this CR. `LiveMigrate` only governs where running workloads go during
KubeVirt upgrades -- with node-local `local-ssd-nvme` volumes there is
nowhere to migrate to, so upgrade-time VMIs restart on this single node.

## Upgrades (chart bump)

Upgrades are supported only N-1 -> N, never skip a minor. Our CR sets no
`spec.imageTag`, so the operand locks to the operator version: the operator
roll is the upgrade.

1. Daily check PR bumps `projects/helm-kubevirt` (Chart.yaml `version` +
   `appVersion`); the publish workflow fetches the new asset and pushes OCI.
2. Bump the wrapper `ref.tag` in `controllers/base/kubevirt-operator.yaml`
   to the new Chart.yaml version (no `$imagepolicy` — atomic hand-bump,
   human merges).
3. RBAC check (mandatory, operator-first): the new operator applies before
   anything reads the CR. `infra-configs` already `dependsOn`
   `infra-controllers`; never reorder.
4. Re-apply the locks onto the fresh CR skeleton (featureGates, smbios,
   workloadUpdateStrategy, no imageTag, monitors unset).

## Deletion order (CRs-first)

Delete the `KubeVirt` CR first and wait for operands to drain, then delete
the operator bundle. Deleting the operator first strands the CR
`Terminating` behind its finalizer (recovery: strip the finalizer).
