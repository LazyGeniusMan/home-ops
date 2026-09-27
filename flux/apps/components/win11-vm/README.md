# win11-vm

Windows 11 guest on KubeVirt: 1 replica (`VirtualMachine`, no autostart), UEFI + SecureBoot, Multus secondary net on `multus/lan-dhcp`, root disk via DataVolume on `local-ssd-nvme` (80Gi).

## Power is manual (locked `runStrategy: Manual`)

Flux never starts/stops the guest; use virtctl only while the Flux Kustomization is suspended:

```bash
flux suspend kustomization apps -n win11-vm
virtctl start win11-vm -n win11-vm            # or: stop / restart
flux resume kustomization apps -n win11-vm
```

## Windows 11 ISO import (blocked until staged)

`spec.source` ships as a `https://REPLACE-ME/Win11_xxH2_x64.iso` placeholder so a fresh boot fails closed (CDI import error-loops instead of CrashLooping). The placeholder and any staged ISO URL are never committed (Microsoft licensing); version stays 24H2.

Prestage: download the 24H2 x64 English ISO (+ virtio-win drivers) on a licensed workstation, stage as `Win11_24H2_English_x64.iso` at `https://files.home-ops.yansyah.my.id/iso/`, point `spec.source.http.url` at it locally, and let CDI import into `win11-vm-rootdisk`. Attach via `virtctl vnc` for Setup. Verify with `kubectl -n win11-vm get|describe dv win11-vm-rootdisk`; before staging the import error + `iso-prestage` annotation is the expected fail-closed signal.

## Network

- `default` (masquerade): cluster egress + virtctl console.
- `lan` (bridge → `multus/lan-dhcp`, NAD owned by the `multus` tenant): guest DHCPs from the router at 192.168.1.1.

## Static MAC addresses

Contract (literal + vault mirror, unicast QEMU-OUI `52:54:00`, unique repo-wide): see `../talos-vm/README.md` — values only below.

| env | MAC | vault path |
| --- | --- | --- |
| dev | — (passthrough, router-assigned) | n/a |
| prd | `52:54:00:03:0B:01` | `pass://acme-prd-bdo1-talos-apps-01/win11-vm/lan-mac` |

## Firmware

UEFI (`q35` + `firmware.bootloader.efi`), `secureBoot: true`, `features.smm.enabled: true` (required for SecureBoot, set explicitly). Contrast talos-vm, which disables it.

## Environments

| Env | Patches |
| --- | --- |
| `dev` | singleton; passthrough of `../base` (no static MAC) |
| `prd` | singleton; static `lan` MAC patch only — base specs unchanged |

## Updates

Manual-only ISO (comment-only, `24H2` is a build label not semver; no `$imagepolicy`; see `flux/apps/update-policies/win11-vm.yaml`). Version source: staged ISO behind `spec.source` in `base/win11-vm.yaml` (never committed).
