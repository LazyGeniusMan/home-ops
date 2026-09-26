# multus

Multus v4.3.0 thick-plugin DaemonSet (first-party `helm-multus` OCI chart in
`controllers/base/multus-daemonset.yaml`: `OCIRepository` + `HelmRelease`,
CRD `CreateReplace`) plus the `lan-dhcp`
NetworkAttachmentDefinition (`configs/base/lan-dhcp.yaml`). Thick plugin:
KubeVirt secondary-net (`NetworkBindingPlugins`) requires the per-node
`multus-daemon`.

## Source: first-party chart, not vendored

No official upstream chart — `projects/helm-multus` wraps the release asset
(`ci/fetch.sh` stages
`https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/v4.3.0/deployments/multus-daemonset-thick.yml`
at publish time and re-applies the two local narrowings as script steps:
ClusterRole narrowing + image re-pin to
`ghcr.io/k8snetworkplumbingwg/multus-cni:v4.3.0-thick`; no YAML committed in
the chart). Flux consumes the published OCI artifact
(`oci://ghcr.io/lazygeniusman/home-ops/projects/helm-multus`, `ref.tag` =
Chart.yaml `version`). The `kube-system` namespaces are upstream's and
correct as-is.

## Node-file residue

The thick install writes node files outside the DaemonSet lifecycle; deleting
the DaemonSet does not clean `/opt/cni/bin`, `/etc/cni/net.d`, `/run`
(sockets), or `/var/lib/cni/multus` — sweep manually on full uninstall or
re-install. The `NetworkAttachmentDefinition` CRD ships in this bundle: full
uninstall = delete NADs first, then the DaemonSet, then the CRD explicitly.

## Bumps

Daily check PR bumps `projects/helm-multus` (Chart.yaml `version` +
`appVersion`); the publish workflow fetches the new asset, re-applies the
narrowings, and pushes OCI. Bump the wrapper `ref.tag` in
`controllers/base/multus-daemonset.yaml` to the new Chart.yaml version (no
`$imagepolicy` — atomic hand-bump, human merges). Re-verify `kube-system`
namespaces, DaemonSet mounts, and the initContainer `-t thick` arg on every
bump; `privileged: true` stays (CNI moves host netns interfaces — hardening
it would break the daemon).

## The `lan-dhcp` contract (single-writer)

This component owns `NetworkAttachmentDefinition/lan-dhcp`; `win11-vm` and
`talos-vm` reference it and must not define their own copy. The NAD carries no
`metadata.namespace` (the infra tenant applies configs with `targetNamespace:
multus`); consumers reference it namespace-qualified as `multus/lan-dhcp`.
L2: `macvlan` on the LAN uplink in `bridge` mode; guests DHCP against the
router at 192.168.1.1. Base carries the `__NODE_NIC__` placeholder; the dev
overlay targets `ens18` (QEMU/KVM virtio, same NIC as the Talos dev
Layer2VIP link and the Cilium dev L2AnnouncementPolicy) and prd targets
`enp45s0`. Secondary interfaces get addresses from the
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
ServiceMonitors wait for the monitoring stack. Bumps: Chart.yaml +
wrapper `ref.tag` together (no ImagePolicy/marker, atomic hand-bump),
human merges.
