# multus

Multus v4.3.0 thick-plugin DaemonSet (first-party `helm-multus` OCI chart in `controllers/base/multus-daemonset.yaml`: `OCIRepository` + `HelmRelease`, CRD `CreateReplace`) plus the `lan-dhcp` NetworkAttachmentDefinition (`configs/base/lan-dhcp.yaml`). Thick plugin: KubeVirt secondary-net (`NetworkBindingPlugins`) requires the per-node `multus-daemon`. `projects/helm-multus` wraps the release asset at publish time (no YAML committed); `ref.tag` = Chart.yaml `version`. `privileged: true` stays (CNI moves host netns interfaces).

## The `lan-dhcp` contract (single-writer)

This component owns `NetworkAttachmentDefinition/lan-dhcp`; `win11-vm` and `talos-vm` reference it as `multus/lan-dhcp` and must not define their own copy. L2: `macvlan` on the LAN uplink in `bridge` mode; guests DHCP against the router at 192.168.1.1 (no in-cluster IPAM — if guests fail to get an address, check the router's DHCP pool first). Base carries the `__NODE_NIC__` placeholder; dev targets `ens18`, prd targets `enp45s0`.

## Node-file residue

The thick install writes node files outside the DaemonSet lifecycle (`/opt/cni/bin`, `/etc/cni/net.d`, `/run`, `/var/lib/cni/multus`) — sweep manually on full uninstall. Full uninstall = delete NADs first, then the DaemonSet, then the CRD explicitly.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | DaemonSet per-node (N/A) |
| `prd` | DaemonSet per-node (N/A) |

Controllers track `../base` with no patches in both envs.

## Updates

Chart.yaml + wrapper `ref.tag` together (no ImagePolicy/marker, atomic hand-bump), human merges. Re-verify namespaces, DaemonSet mounts, and the initContainer `-t thick` arg on every bump.
