# multus

Multus v4.3.0 thick-plugin DaemonSet
(`controllers/base/multus-daemonset.yaml`) plus the `lan-dhcp`
NetworkAttachmentDefinition (`configs/base/lan-dhcp.yaml`).

## Source: vendored, not charted

No official upstream chart -- vendored pinned release manifest:

- Upstream:
  `https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/v4.3.0/deployments/multus-daemonset-thick.yml`.
- Image pin: both image fields (daemon + `install-multus-binary` init
  container) pinned to `ghcr.io/k8snetworkplumbingwg/multus-cni:v4.3.0-thick`
  (upstream tags the DaemonSet `snapshot-thick`, which floats -- never
  track it).
- The `kube-system` ServiceAccount/ClusterRole(Binding)/ConfigMap/DaemonSet
  namespaces are upstream's and correct as-is: cluster-scoped resources are
  untouched by the tenant `targetNamespace`, and the multus namespace is
  created by the tenant entry.

Thick plugin: KubeVirt secondary-net (`NetworkBindingPlugins`) requires the
per-node `multus-daemon`.

## Node-file residue

The thick install writes node files outside the DaemonSet lifecycle (via
the initContainer + daemon mounts). Deleting the DaemonSet does not clean
them; sweep manually on full uninstall or re-install:

- `/opt/cni/bin` (`multus-shim` binary).
- `/etc/cni/net.d` (incl. `00-multus.conf`).
- `/run` (sockets incl. `/run/k8s.cni.cncf.io`, `/run/netns`, `/run/multus`).
- `/var/lib/cni/multus` (daemon state).

The `NetworkAttachmentDefinition` CRD ships in this bundle and is not
removed by deleting the DaemonSet. Full uninstall = delete NADs first,
then the DaemonSet, then the CRD explicitly.

## Re-download

1. Re-download the upstream URL at the new tag, diff against
   `controllers/base/multus-daemonset.yaml`.
2. Re-pin both images together (daemon + initContainer) to
   `ghcr.io/k8snetworkplumbingwg/multus-cni:<tag>-thick` -- never
   `snapshot-thick`, never a half-pinned pair.
3. Re-verify: `kube-system` ServiceAccount / ClusterRole(Binding) /
   ConfigMap namespaces, DaemonSet mounts, initContainer `-t thick` arg.
4. Bump the `$imagepolicy` marker (`infra:multus:tag`,
   `update-policies/multus.yaml >=4.3.0`); ImageUpdateAutomation opens the
   PR, human merges. `infra-configs` `dependsOn` `infra-controllers` is
   untouched.

## The `lan-dhcp` contract (single-writer)

This component owns `NetworkAttachmentDefinition/lan-dhcp`; `win11-vm` and
`talos-vm` reference it and must not define their own copy.

- The NAD carries no `metadata.namespace`: the infra tenant applies configs
  with `targetNamespace: multus`, so it lands there.
- Consumers reference it namespace-qualified as `multus/lan-dhcp`
  (KubeVirt `spec.template.spec.networks[].multus.networkName`, also via
  the `k8s.v1.cni.cncf.io/networks` annotation).
- L2: `macvlan` on the LAN uplink in `bridge` mode; guests DHCP against
  the router at 192.168.1.1. Base carries the `__NODE_NIC__` placeholder;
  both overlays target host NIC `enp45s0`.

Secondary interfaces get addresses from the LAN router's DHCP (no in-cluster
IPAM). If guests fail to get an address, check the router's DHCP pool first.

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
