# win11-vm (§13.4)

Windows 11 guest on KubeVirt: 1 replica (`VirtualMachine`, no autostart),
UEFI + SecureBoot, Multus secondary net on `multus/lan-dhcp`, root disk via
DataVolume on `local-ssd-nvme`.

## Power is manual — runbook (LOCKED `runStrategy: Manual`)

`runStrategy: Manual` means Flux reconciliation NEVER starts/stops the guest:
power survives reconciliation by design. Operate power with virtctl, but FIRST
suspend the Flux Kustomization so a coincident reconcile cannot fight you:

```bash
flux suspend kustomization apps -n win11-vm   # pause reconciliation
virtctl start win11-vm -n win11-vm            # or: virtctl stop / restart
flux resume kustomization apps -n win11-vm    # re-enable reconciliation
```

The VM object itself stays managed by Flux at all times (spec edits still
flow through git); only the power state is yours.

## Windows 11 ISO import (documented prerequisite)

The manifest ships the DataVolume shape with a PLACEHOLDER `spec.source` —
Microsoft licensing means the ISO URL is never committed. To provision:

1. Download the Windows 11 ISO (`Win11_24H2_English_x64.iso` or newer) from
   Microsoft on a licensed workstation.
2. Stage it on local infra (e.g. `https://files.home-ops.yansyah.my.id/iso/`
   or a PVC upload via `virtctl image-upload`), plus the
   [virtio-win](https://github.com/virtio-win/virtio-win-pkg-scripts) driver
   ISO for the `virtio` disk/NIC during Setup.
3. Fill `spec.source` in `base/win11-vm.yaml` (`http.url` or switch to a
   `pvc` source pointing at the uploaded golden image), commit, let Flux
   reconcile — CDI imports the ISO into `win11-vm-rootdisk` (80Gi,
   `local-ssd-nvme`).
4. Attach via VNC/SPICE (`virtctl vnc`) to run Windows Setup; install
   virtio-win drivers when the disk is not visible.

## Network

- `default` (masquerade/pod): cluster egress + virtctl console.
- `lan` (bridge → `multus/lan-dhcp`, namespace-qualified: the NAD lives in
  the `multus` tenant namespace — see multus README single-writer contract).
  The guest DHCPs against the router at 192.168.1.1.

## Firmware

UEFI (`q35` + `firmware.bootloader.efi`) with `secureBoot: true` and
`features.smm.enabled: true` (SMM is required for SecureBoot and is NOT
auto-enabled — set explicitly). Contrast talos-vm, which disables it.

## Environments

`production` and `staging` track `../base` with no patches.

## Telemetry-off / monitoring / updates

- No guest-agent reporting is configured; Windows telemetry is out of scope
  for Flux (harden in the image/Setup, not here).
- No ServiceMonitors (KubeVirt VM metrics flow via kubevirt infra when the
  monitoring stack lands).
- This VM tracks its base image/ISO, not a chart: the `$imagepolicy` marker
  (`apps:win11-vm:tag`) anchors the ISO annotation. When the staged ISO is
  refreshed, update the source + marker so update-automation opens a PR.
