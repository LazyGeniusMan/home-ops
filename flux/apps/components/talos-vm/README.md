# talos-vm (§13.5)

Talos Linux guest on KubeVirt: 1 replica (`VirtualMachine`, no autostart),
UEFI with SecureBoot DISABLED, Multus secondary net on `multus/lan-dhcp`,
root disk via DataVolume on `local-ssd-nvme`.

## Power is manual — runbook (LOCKED `runStrategy: Manual`)

Same contract as win11-vm: Flux NEVER changes guest power; operate with
virtctl only while the Flux Kustomization is suspended:

```bash
flux suspend kustomization apps -n talos-vm   # pause reconciliation
virtctl start talos-vm -n talos-vm            # or: virtctl stop / restart
flux resume kustomization apps -n talos-vm    # re-enable reconciliation
```

## Talos nocloud ISO source (aligned to talos-docs v1.14)

- `dataVolumeTemplates[].spec.source.http.url` points at the Image Factory
  nocloud artifact for amd64 / v1.14
  (`https://factory.talos.dev/image/nocloud/amd64/v1.14/nocloud-amd64.iso`;
  shape per `getting-started.mdx`: "download the ISO for your architecture
  from the Image factory"). Pin to the exact v1.14 factory URL in use and
  refresh it with Talos minor bumps.
- Guest-image alignment: the factory schematic for this VM needs NO extra
  system extensions — virtio disk/NIC are in-tree in Talos, and the host
  schematics (`talos/clusters/*/schematics.yml`) already cover
  intel-ucode/i915/realtek-firmware/netbird at the host level. Optional:
  add the `qemu-guest-agent` extension via a factory schematic and refresh
  the URL above if guest-agent integration is wanted.
- Bootstrap the guest with `talosctl apply-config` over the `lan` interface
  address it DHCPs from the router (192.168.1.1), same as any bare-metal
  Talos node in maintenance mode.

## Firmware (contrast win11-vm)

UEFI (`q35` + `firmware.bootloader.efi`) with `secureBoot: false` and NO
`features.smm` block — the Talos nocloud image is not SecureBoot-signed, so
win11-vm's SecureBoot+SMM shape would refuse to boot here.

## Network

- `default` (masquerade/pod): cluster egress + virtctl console.
- `lan` (bridge → `multus/lan-dhcp`, namespace-qualified: the NAD lives in
  the `multus` tenant namespace — see multus README single-writer contract).
  The guest DHCPs against the router at 192.168.1.1.

## Static MAC addresses

The `lan` bridge interface carries a static MAC per env so the router
(192.168.1.1) hands each VM a stable DHCP lease / reservation. The MAC lives
only as a literal on the per-env overlay patch (`macAddress` is a plain
string field — no ESO, no Secret); Proton Pass holds a manual mirror of the
same value (`lan-mac` item) for the operator to read when creating the router
DHCP reservation.

Safety rules for every MAC below:

- Unicast: first-octet LSB is 0 — never multicast/broadcast.
- QEMU-OUI `52:54:00` prefix (KubeVirt-assigned range, no clash with real
  NICs).
- Unique per L2: dev+stg+prd share 192.168.1.0/24 with win11-vm, so every
  static MAC repo-wide must differ.

| env | MAC | vault path |
| --- | --- | --- |
| dev | `52:54:00:01:0A:01` | `pass://acme-dev-bdo1-talos-apps-01/talos-vm/lan-mac` |
| stg | `52:54:00:02:0A:01` | `pass://acme-prd-bdo1-talos-apps-01/talos-vm/lan-mac` |
| prd | `52:54:00:03:0A:01` | `pass://acme-prd-bdo1-talos-apps-01/talos-vm/lan-mac` |

Mirror warning: the per-env patch literal and the vault value must agree —
just two copies, nothing else to keep in step. If either side changes, update
both. User action: create a `lan-mac` item holding the env's exact literal
under `<vault>/talos-vm/` in Proton Pass.

## Environments

- `dev`: minimal specs (1 core / 1Gi / 10Gi — boot+DHCP minimum) + static
  `lan` MAC via `talos-vm-dev-patch.yaml`.
- `stg` / `prd`: base specs (2 cores / 4Gi / 20Gi) + static `lan` MAC via
  the per-env patch.

## Telemetry-off / monitoring / updates

- Talos ships no phone-home; no guest reporting is configured here.
- No ServiceMonitors (KubeVirt VM metrics flow via kubevirt infra when the
  monitoring stack lands).
- This VM tracks its base image/ISO: the `$imagepolicy` marker
  (`apps:talos-vm:tag`) anchors the nocloud ISO URL. Refresh the URL on Talos
  minor bumps so update-automation opens a PR.
