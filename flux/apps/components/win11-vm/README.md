# win11-vm (§13.4)

Windows 11 guest on KubeVirt: 1 replica (`VirtualMachine`, no autostart),
UEFI + SecureBoot, Multus secondary net on `multus/lan-dhcp`, root disk via
DataVolume on `local-ssd-nvme`.

## Power is manual — runbook (LOCKED `runStrategy: Manual`)

Flux reconciliation NEVER starts/stops the guest: power survives by design.
Operate power with virtctl, but FIRST suspend the Flux Kustomization so a
coincident reconcile cannot fight you:

```bash
flux suspend kustomization apps -n win11-vm   # pause reconciliation
virtctl start win11-vm -n win11-vm            # or: virtctl stop / restart
flux resume kustomization apps -n win11-vm    # re-enable reconciliation
```

The VM object itself stays managed by Flux (spec edits flow through git);
only the power state is yours.

## Windows 11 ISO import (documented prerequisite)

The manifest ships with a PLACEHOLDER `spec.source` — the ISO URL is never
committed (Microsoft licensing). To provision:

1. Download the Windows 11 ISO (`Win11_24H2_English_x64.iso` or newer) on a
   licensed workstation.
2. Stage it on local infra (e.g. `https://files.home-ops.yansyah.my.id/iso/`
   or a PVC upload via `virtctl image-upload`), plus the
   [virtio-win](https://github.com/virtio-win/virtio-win-pkg-scripts) driver
   ISO for the `virtio` disk/NIC during Setup.
3. Fill `spec.source` in `base/win11-vm.yaml` (`http.url` or a `pvc` source),
   commit, let Flux reconcile — CDI imports the ISO into
   `win11-vm-rootdisk` (80Gi, `local-ssd-nvme`).
4. Attach via VNC/SPICE (`virtctl vnc`) to run Windows Setup; install
   virtio-win drivers when the disk is not visible.

## Network

- `default` (masquerade/pod): cluster egress + virtctl console.
- `lan` (bridge → `multus/lan-dhcp`, NAD owned by the `multus` tenant —
  single-writer, do not define one here). The guest DHCPs from the router
  at 192.168.1.1.

## Static MAC addresses

`prd` carries a static MAC on the `lan` interface for a stable router DHCP
lease (`dev` is a passthrough with no static MAC). The MAC is a literal on
the per-env patch (`macAddress` is a plain string — no ESO); Proton Pass
holds a manual mirror (`lan-mac` item) for the router reservation. Patch
literal and vault value must agree — update both together. MACs must be
unicast, QEMU-OUI `52:54:00` prefixed, and unique repo-wide (dev+prd share
192.168.1.0/24 with talos-vm).

| env | MAC | vault path |
| --- | --- | --- |
| dev | — (passthrough, router-assigned) | n/a |
| prd | `52:54:00:03:0B:01` | `pass://acme-prd-bdo1-talos-apps-01/win11-vm/lan-mac` |

## Firmware

UEFI (`q35` + `firmware.bootloader.efi`) with `secureBoot: true` and
`features.smm.enabled: true` (SMM is required for SecureBoot and is NOT
auto-enabled — set explicitly). Contrast talos-vm, which disables it.

## Environments

| Env | Patches |
| --- | --- |
| `dev` | singleton; passthrough of `../base` (no static MAC) |
| `prd` | singleton; static `lan` MAC patch — base specs (4 cores / 8Gi / 80Gi) unchanged |

Upstream reference (read-only): `/tmp/home-ops-docs/kubevirt-docs` (guest shape; ISO itself is licensed, never committed).

## Telemetry-off / monitoring / updates

- No guest-agent reporting; Windows telemetry is out of scope for Flux
  (harden in the image/Setup). No ServiceMonitors.
- The `$imagepolicy` marker (`apps:win11-vm:tag`) anchors the ISO — update
  source + marker together on refresh so update-automation opens a PR.

## Upgrade runbook

- Version source: the staged ISO behind `spec.source` in
  `base/win11-vm.yaml` (licensed Microsoft image, never committed).
- Changelog: n/a (no public feed; track the Windows 11 release notes for
  the build in use).
- Bump: stage the new ISO on local infra, point `spec.source` at it, move
  the `$imagepolicy` marker (`apps:win11-vm:tag`,
  `update-policies/win11-vm.yaml`) in the same commit, and record the
  build in this README.
- Migrate: power is manual (`runStrategy: Manual` — virtctl runbook above
  with the Kustomization suspended). Verify: the guest boots, DHCPs a
  `lan` address, and virtio drivers stay healthy.
