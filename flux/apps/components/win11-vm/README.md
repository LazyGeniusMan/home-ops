# win11-vm

Windows 11 guest on KubeVirt: 1 replica (`VirtualMachine`, no autostart),
UEFI + SecureBoot, Multus secondary net on `multus/lan-dhcp`, root disk via
DataVolume on `local-ssd-nvme`.

## Power is manual (LOCKED `runStrategy: Manual`)

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
committed (Microsoft licensing). Provision by downloading the Windows 11 ISO
on a licensed workstation, staging it on local infra (or a PVC upload via
`virtctl image-upload`, plus the virtio-win driver ISO), filling
`spec.source` in `base/win11-vm.yaml`, and committing — CDI imports the ISO
into `win11-vm-rootdisk` (80Gi, `local-ssd-nvme`). Attach via VNC/SPICE
(`virtctl vnc`) for Windows Setup.

## Network

- `default` (masquerade/pod): cluster egress + virtctl console.
- `lan` (bridge → `multus/lan-dhcp`, NAD owned by the `multus` tenant —
  single-writer, do not define one here). The guest DHCPs from the router
  at 192.168.1.1.

## Static MAC addresses

`prd` carries a static MAC on the `lan` interface for a stable router DHCP
lease (`dev` is a passthrough with no static MAC). Contract (literal +
vault mirror, unicast QEMU-OUI `52:54:00`, unique repo-wide): see
`../talos-vm/README.md` — values only below.

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
  source + marker together on refresh so update-automation opens a PR, and
  record the build here. Version source: the staged ISO behind `spec.source`
  in `base/win11-vm.yaml` (licensed Microsoft image, never committed).
