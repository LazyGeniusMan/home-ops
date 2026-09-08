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

## Environments

`production` and `staging` track `../base` with no patches.

## Telemetry-off / monitoring / updates

- Talos ships no phone-home; no guest reporting is configured here.
- No ServiceMonitors (KubeVirt VM metrics flow via kubevirt infra when the
  monitoring stack lands).
- This VM tracks its base image/ISO: the `$imagepolicy` marker
  (`apps:talos-vm:tag`) anchors the nocloud ISO URL. Refresh the URL on Talos
  minor bumps so update-automation opens a PR.
