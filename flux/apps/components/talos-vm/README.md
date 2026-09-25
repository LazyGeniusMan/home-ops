# talos-vm (§13.5)

Talos Linux guest on KubeVirt: 1 replica (`VirtualMachine`, no autostart),
UEFI with SecureBoot DISABLED, Multus secondary net on `multus/lan-dhcp`,
root disk via DataVolume on `local-ssd-nvme`.

## Power is manual — runbook (LOCKED `runStrategy: Manual`)

Flux NEVER changes guest power; operate with virtctl only while the Flux
Kustomization is suspended:

```bash
flux suspend kustomization apps -n talos-vm   # pause reconciliation
virtctl start talos-vm -n talos-vm            # or: virtctl stop / restart
flux resume kustomization apps -n talos-vm    # re-enable reconciliation
```

## Talos nocloud ISO source (aligned to the Talos version in `talos/ansible/group_vars/all.yml`)

- `dataVolumeTemplates[].spec.source.http.url` points at the Image Factory
  nocloud artifact for amd64 / v1.15.0-alpha.0
  (`https://factory.talos.dev/image/nocloud/amd64/v1.15.0-alpha.0/nocloud-amd64.iso`).
  Refresh it with Talos minor bumps.
- The factory schematic needs NO extra system extensions (virtio disk/NIC
  are in-tree; host schematics already cover
  intel-ucode/i915/realtek-firmware/netbird). Optional: add the
  `qemu-guest-agent` extension via a factory schematic and refresh the URL.
- Bootstrap the guest with `talosctl apply-config` over the `lan` address
  it DHCPs from the router (192.168.1.1).

## Firmware (contrast win11-vm)

UEFI (`q35` + `firmware.bootloader.efi`) with `secureBoot: false` and NO
`features.smm` block — the Talos nocloud image is not SecureBoot-signed.

## Network

- `default` (masquerade/pod): cluster egress + virtctl console.
- `lan` (bridge → `multus/lan-dhcp`, NAD owned by the `multus` tenant —
  single-writer, do not define one here). The guest DHCPs from the router
  at 192.168.1.1.

## Static MAC addresses

The `lan` interface carries a static MAC per env for a stable router DHCP
lease. The MAC is a literal on the per-env patch (`macAddress` is a plain
string — no ESO); Proton Pass holds a manual mirror (`lan-mac` item) for
the router reservation. Patch literal and vault value must agree — update
both together. MACs must be unicast, QEMU-OUI `52:54:00` prefixed, and
unique repo-wide (dev+prd share 192.168.1.0/24 with win11-vm).

| env | MAC | vault path |
| --- | --- | --- |
| dev | `52:54:00:01:0A:01` | `pass://acme-dev-bdo1-talos-apps-01/talos-vm/lan-mac` |
| prd | `52:54:00:03:0A:01` | `pass://acme-prd-bdo1-talos-apps-01/talos-vm/lan-mac` |

## Environments

| Env | Patches |
| --- | --- |
| `dev` | singleton; minimal specs (1 core / 1Gi / 10Gi — boot+DHCP minimum) + static `lan` MAC |
| `prd` | singleton; base specs (2 cores / 4Gi / 20Gi) + static `lan` MAC |

Upstream reference (read-only): `/tmp/home-ops-docs/talos-docs` (guest image/ISO shape).

## Telemetry-off / monitoring / updates

- Talos ships no phone-home; no guest reporting is configured here.
- No ServiceMonitors (KubeVirt VM metrics flow via kubevirt infra).
- The `$imagepolicy` marker (`apps:talos-vm:tag`) anchors the nocloud ISO
  URL. Refresh the URL on Talos minor bumps so update-automation opens a PR.

## Upgrade runbook

- Version source: the nocloud ISO URL in `base/talos-vm.yaml`
  (v1.15.0-alpha.0) — MUST stay aligned to the Talos version in
  `talos/ansible/group_vars/all.yml`.
- Changelog: https://github.com/siderolabs/talos/releases.
- Bump: refresh the factory nocloud URL + move the `$imagepolicy` marker
  (`apps:talos-vm:tag`, `update-policies/talos-vm.yaml`) together on Talos
  minor bumps so update-automation opens the PR.
- Migrate: power is manual (`runStrategy: Manual` — virtctl runbook above).
  Verify: the guest DHCPs a `lan` address and `talosctl health` passes.
