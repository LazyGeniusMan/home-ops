# talos-vm

Talos Linux guest on KubeVirt: 1 replica (`VirtualMachine`, no autostart), UEFI with SecureBoot disabled, Multus secondary net on `multus/lan-dhcp`, root disk via DataVolume on `local-ssd-nvme`.

## Power is manual (locked `runStrategy: Manual`)

Flux never changes guest power; use virtctl only while the Flux Kustomization is suspended:

```bash
flux suspend kustomization apps -n talos-vm
virtctl start talos-vm -n talos-vm            # or: stop / restart
flux resume kustomization apps -n talos-vm
```

## ISO source

`dataVolumeTemplates[].spec.source.http.url` points at the Image Factory nocloud artifact (amd64, v1.15.0-alpha.0) — refresh on Talos minor bumps, aligned to `talos/ansible/group_vars/all.yml`. Factory schematic needs no extra extensions (virtio in-tree); optional `qemu-guest-agent` via a factory schematic. Bootstrap with `talosctl apply-config` over the `lan` DHCP address (router 192.168.1.1).

## Firmware

UEFI (`q35` + `firmware.bootloader.efi`), `secureBoot: false`, no `features.smm` — the Talos nocloud image is not SecureBoot-signed (contrast win11-vm).

## Network

- `default` (masquerade): cluster egress + virtctl console.
- `lan` (bridge → `multus/lan-dhcp`, NAD owned by the `multus` tenant): guest DHCPs from the router at 192.168.1.1.

## Static MAC addresses (canonical contract)

`lan` carries a static MAC per env (literal on the per-env patch; vault holds a manual `lan-mac` mirror for the router reservation — update both together). Unicast, QEMU-OUI `52:54:00`, unique repo-wide (win11-vm points here for the contract).

| env | MAC | vault path |
| --- | --- | --- |
| dev | `52:54:00:01:0A:01` | `pass://acme-dev-bdo1-talos-apps-01/talos-vm/lan-mac` |
| prd | `52:54:00:03:0A:01` | `pass://acme-prd-bdo1-talos-apps-01/talos-vm/lan-mac` |

## Environments

| Env | Patches |
| --- | --- |
| `dev` | singleton; minimal specs (1 core / 1Gi / 10Gi) + static `lan` MAC |
| `prd` | singleton; base specs (2 cores / 4Gi / 20Gi) + static `lan` MAC |

## Updates

Manual-only ISO (no `$imagepolicy`; see `flux/apps/update-policies/talos-vm.yaml`). Version source: nocloud ISO URL in `base/talos-vm.yaml`. Changelog: [talos](https://github.com/siderolabs/talos/releases).
