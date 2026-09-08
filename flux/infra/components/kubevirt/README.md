# kubevirt (§13.2)

KubeVirt v1.9.0: vendored pinned `kubevirt-operator.yaml` (controllers) +
`KubeVirt` CR (configs) enabling virt-operator/api/controller.

## Talos prerequisites — verified (§13.2 gate)

Bounded check against `talos-docs` v1.14 + this repo's schematics (host is a
bare-metal MSI Cubi, so `/dev/kvm` is expected):

- `advanced-guides/install-kubevirt.mdx`: requires virtualization enabled in
  BIOS (host is bare metal → KVM present; nested-virt only matters for VMs),
  optional bridge-mode NIC for Multus (§13.1 uses macvlan, no bridge needed),
  `local-path-provisioner` for CDI scratch space (present, §9.5), and shared
  storage (NFS/Longhorn) only for LiveMigration (single-node: not required).
- `virtualized-platforms/kvm.mdx` confirms KVM needs `/dev/kvm` on the host —
  satisfied by bare metal; no Talos schematic change needed (vanilla
  schematic, `_base/schematics.yml`; extensions are intel-ucode/i915/
  realtek-firmware/netbird — none KVM-related because Talos ships KVM in the
  kernel already).
- Alignment with schematics: no `siderolabs/kvm`-style extension exists or is
  needed; the only KubeVirt-relevant Talos surface is the storage + network
  already covered by §9.5 local-path and §13.1.

Result: **prerequisites satisfied, no Talos config change required.**
Day-0 check: `ls /dev/kvm` on the host before first VM start; if nested-virt
ever matters (dev cluster under QEMU/KVM), enable it in the host BIOS.

## Source: vendored, not charted

No official upstream chart — pinned release manifests, same Gitless pattern
as gateway-api/multus:

- Operator:
  `https://github.com/kubevirt/kubevirt/releases/download/v1.9.0/kubevirt-operator.yaml`
- CR shape:
  `https://github.com/kubevirt/kubevirt/releases/download/v1.9.0/kubevirt-cr.yaml`
  (upstream CR is a near-empty skeleton; ours extends it per the Talos guide —
  see header in `configs/base/kubevirt-cr.yaml`).

## The CR at a glance

- `featureGates: [LiveMigration, NetworkBindingPlugins]` (Multus needs the
  latter); `useEmulation: false` — real KVM on bare metal.
- `smbios` TalosCloud identity block straight from the upstream Talos guide.
- `workloadUpdateStrategy: [LiveMigrate]` (guide default).
- Storage: default-class `local-ssd-nvme` used implicitly (the v1.9.0 CRD
  schema has no top-level storage/scratch knob — verified).
- Metrics on where safe (upstream prometheus annotations only, no
  ServiceMonitor pinned); telemetry off — upstream exposes no phone-home
  flags, so there is nothing to switch off.

## Unguarded monitors — flip note

`monitorNamespace` / `monitorAccount` / `serviceMonitorNamespace` are
deliberately UNSET: their defaults (`openshift-monitor` / `prometheus-k8s`)
do not exist here, and pinning them now would create dangling RBAC. WHEN the
monitoring stack lands (§14): set `monitorNamespace` + `monitorAccount` to
the Prometheus namespace/account and `serviceMonitorNamespace` to where the
ServiceMonitors should live, then verify `virt-operator` logs show successful
ServiceMonitor creation.

## Environments

`prd` and `stg` (controllers + configs) track `../base` with no
patches — a single-node fleet has one KVM host profile.

## runStrategy ownership (coordination with §13.4/§13.5)

Guest power state is owned by the VM manifests (`runStrategy: Manual`), NOT
by this CR. `LiveMigrate` here only governs where running workloads go
during KubeVirt upgrades — with node-local `local-ssd-nvme` volumes (no
shared storage) there is nowhere to migrate TO, so expect upgrade-time
VMIs to restart rather than live-migrate on this single node.

## Telemetry-off / monitoring / updates

- No reporting knobs upstream (verified against the v1.9.0 CRD: no
  `telemetry` key) — nothing to disable.
- Component metrics via prometheus annotations; ServiceMonitors deferred to
  the monitoring stack (flip note above).
- Version bumps via `update-policies/kubevirt.yaml` (`>=1.9.0`) → PR
  automation; the `$imagepolicy` marker lives in the headers of both vendored
  files. Update procedure: re-download both URLs at the new tag, diff, then
  bump the marker.
