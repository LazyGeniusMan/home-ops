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

## Static MAC addresses

`stg` and `prd` carry a static MAC on the `lan` bridge interface so the
router (192.168.1.1) hands each VM a stable DHCP lease / reservation (`dev`
is a passthrough with no static MAC). The MAC lives only as a literal on the
per-env overlay patch (`macAddress` is a plain string field — no ESO, no
Secret); Proton Pass holds a manual mirror of the same value (`lan-mac`
item) for the operator to read when creating the router DHCP reservation.

Safety rules for every MAC below:

- Unicast: first-octet LSB is 0 — never multicast/broadcast.
- QEMU-OUI `52:54:00` prefix (KubeVirt-assigned range, no clash with real
  NICs).
- Unique per L2: dev+stg+prd share 192.168.1.0/24 with talos-vm, so every
  static MAC repo-wide must differ.

| env | MAC | vault path |
| --- | --- | --- |
| dev | — (passthrough, router-assigned) | n/a |
| stg | `52:54:00:02:0B:01` | `pass://acme-prd-bdo1-talos-apps-01/win11-vm/lan-mac` |
| prd | `52:54:00:03:0B:01` | `pass://acme-prd-bdo1-talos-apps-01/win11-vm/lan-mac` |

Mirror warning: the per-env patch literal and the vault value must agree —
just two copies, nothing else to keep in step. If either side changes, update
both. User action: create a `lan-mac` item holding the env's exact literal
under `<vault>/win11-vm/` in Proton Pass.

## Firmware

UEFI (`q35` + `firmware.bootloader.efi`) with `secureBoot: true` and
`features.smm.enabled: true` (SMM is required for SecureBoot and is NOT
auto-enabled — set explicitly). Contrast talos-vm, which disables it.

## Environments

`dev` is a passthrough of `../base` (no static MAC); `stg` and `prd` add a
static `lan` MAC patch each — base specs (4 cores / 8Gi / 80Gi) unchanged.

## Telemetry-off / monitoring / updates

- No guest-agent reporting is configured; Windows telemetry is out of scope
  for Flux (harden in the image/Setup, not here).
- No ServiceMonitors (KubeVirt VM metrics flow via kubevirt infra when the
  monitoring stack lands).
- This VM tracks its base image/ISO, not a chart: the `$imagepolicy` marker
  (`apps:win11-vm:tag`) anchors the ISO annotation. When the staged ISO is
  refreshed, update the source + marker so update-automation opens a PR.
