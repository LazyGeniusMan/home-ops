# multus

Multus v4.3.0 thick-plugin DaemonSet
(`controllers/base/multus-daemonset.yaml`) plus the `lan-dhcp`
NetworkAttachmentDefinition (`configs/base/lan-dhcp.yaml`). Thick plugin:
KubeVirt secondary-net (`NetworkBindingPlugins`) requires the per-node
`multus-daemon`.

## Source: vendored, not charted

No official upstream chart — vendored pinned release manifest from
`https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/v4.3.0/deployments/multus-daemonset-thick.yml`.
Both image fields (daemon + `install-multus-binary` init container) pin to
`ghcr.io/k8snetworkplumbingwg/multus-cni:v4.3.0-thick` (upstream tags the
DaemonSet `snapshot-thick`, which floats — never track it). The `kube-system`
ServiceAccount/ClusterRole(Binding)/ConfigMap/DaemonSet namespaces are
upstream's and correct as-is.

## Node-file residue

The thick install writes node files outside the DaemonSet lifecycle; deleting
the DaemonSet does not clean `/opt/cni/bin`, `/etc/cni/net.d`, `/run`
(sockets), or `/var/lib/cni/multus` — sweep manually on full uninstall or
re-install. The `NetworkAttachmentDefinition` CRD ships in this bundle: full
uninstall = delete NADs first, then the DaemonSet, then the CRD explicitly.

## Re-download

Re-download the upstream URL at the new tag and diff against
`controllers/base/multus-daemonset.yaml`; re-pin both images together (never
`snapshot-thick`, never a half-pinned pair); re-verify `kube-system` namespaces,
DaemonSet mounts, and the initContainer `-t thick` arg. Bump the `$imagepolicy`
marker (`infra:multus:tag`, `update-policies/multus.yaml >=4.3.0`);
ImageUpdateAutomation opens the PR, human merges.

## The `lan-dhcp` contract (single-writer)

This component owns `NetworkAttachmentDefinition/lan-dhcp`; `win11-vm` and
`talos-vm` reference it and must not define their own copy. The NAD carries no
`metadata.namespace` (the infra tenant applies configs with `targetNamespace:
multus`); consumers reference it namespace-qualified as `multus/lan-dhcp`.
L2: `macvlan` on the LAN uplink in `bridge` mode; guests DHCP against the
router at 192.168.1.1. Base carries the `__NODE_NIC__` placeholder; both
overlays target host NIC `enp45s0`. Secondary interfaces get addresses from the
LAN router's DHCP (no in-cluster IPAM) — if guests fail to get an address, check
the router's DHCP pool first.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | DaemonSet per-node (N/A) |
| `prd` | DaemonSet per-node (N/A) |

Controllers track `../base` with no patches in both envs.

## Telemetry / monitoring / updates

No reporting knobs upstream. Metrics via plain prometheus annotations;
ServiceMonitors wait for the monitoring stack. Bumps via
`update-policies/multus.yaml` -> PR automation (marker in the header of
`controllers/base/multus-daemonset.yaml`).
