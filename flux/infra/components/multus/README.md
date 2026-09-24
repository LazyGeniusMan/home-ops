# multus (§13.1)

Multus v4.3.0 thick-plugin DaemonSet (`controllers/base/multus-daemonset.yaml`)
plus the `lan-dhcp` NetworkAttachmentDefinition (`configs/base/lan-dhcp.yaml`).

## Source: vendored, not charted

No official upstream chart exists — vendored pinned release manifest +
flux-pushed OCI artifact:

- Upstream:
  `https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/v4.3.0/deployments/multus-daemonset-thick.yml`
  (v4.3.0 GitHub release).
- Image pin: upstream tags the DaemonSet `snapshot-thick` (floating); both
  image fields (daemon + install-multus-binary init container) are pinned to
  `ghcr.io/k8snetworkplumbingwg/multus-cni:v4.3.0-thick`.
- The `kube-system` ServiceAccount/ClusterRole(Binding)/ConfigMap/DaemonSet
  namespaces are upstream's and are correct as-is: infra-controllers applies
  with targetNamespace=multus but cluster-scoped resources (CRD, ClusterRole,
  ClusterRoleBinding) are untouched by it, and the multus namespace is created
  by the tenant entry.

## Plugin mode

Thick plugin: KubeVirt secondary-net (§13.2 `NetworkBindingPlugins`)
requires the per-node `multus-daemon`.

## Node-file residue + CRD-on-uninstall

The thick install writes node files OUTSIDE the DaemonSet lifecycle (via
the `install-multus-binary` initContainer + daemon mounts). Deleting the
DaemonSet does NOT clean them; a manual sweep is required on full
uninstall or re-install, or stale CNI files break pod networking:

- `/opt/cni/bin` — installed `multus-shim` binary (hostPath `cnibin`).
- `/etc/cni/net.d` — generated configs incl. `00-multus.conf` (hostPath
  `cni`).
- `/run` — sockets incl. `/run/k8s.cni.cncf.io`, `/run/netns`,
  `/run/multus` (hostPaths `host-run`, `host-run-k8s-cni-cncf-io`,
  `host-run-netns`).
- `/var/lib/cni/multus` — daemon state (hostPath
  `host-var-lib-cni-multus`).

The `NetworkAttachmentDefinition` CRD
(`network-attachment-definitions.k8s.cni.cncf.io`) ships in this same
bundle and is NOT removed by deleting the DaemonSet. Full uninstall =
delete NADs first, then the DaemonSet, then the CRD explicitly — leftover
NADs/CRD linger otherwise. No CRD-only Kustomization split is used
(community charts and CRD splits fail the repo bar).

## Re-download runbook

1. Re-download the upstream URL at the new tag
   (`.../multus-cni/v<tag>/deployments/multus-daemonset-thick.yml`), diff
   against `controllers/base/multus-daemonset.yaml`.
2. Re-apply both pins together (dual image-pin discipline): DaemonSet
   container `kube-multus` AND initContainer `install-multus-binary` to
   `ghcr.io/k8snetworkplumbingwg/multus-cni:<tag>-thick` — never
   `snapshot-thick`, never a half-pinned pair.
3. Re-verify the §13.1 surface: `kube-system` ServiceAccount /
   ClusterRole(Binding) / ConfigMap namespaces, DaemonSet mounts above,
   `initContainer` `-t thick` arg.
4. Bump the `$imagepolicy` marker (`infra:multus:tag`,
   `update-policies/multus.yaml >=4.3.0`); ImageUpdateAutomation opens the
   PR, human merges. `infra-configs` `dependsOn` `infra-controllers` is
   untouched — the NAD in configs applies after the DaemonSet/CRD land.

## The `lan-dhcp` contract (single-writer)

This component OWNS `NetworkAttachmentDefinition/lan-dhcp`; `win11-vm` and
`talos-vm` reference it — they must not define their own copy.

- The NAD carries NO `metadata.namespace`: the infra tenant applies configs
  with `targetNamespace: << inputs.tenant >>` (= `multus`), so it lands in
  the `multus` namespace. Verified: Flux `targetNamespace` rewrites the
  namespace of every namespaced resource even when one is already set, so
  setting one here would only duplicate what the tenant does.
- Consumers MUST reference it namespace-qualified as `multus/lan-dhcp`
  (KubeVirt `spec.template.spec.networks[].multus.networkName`, also honored
  via the `k8s.v1.cni.cncf.io/networks` annotation).
- L2: `macvlan` on the LAN uplink in `bridge` mode; guests DHCP directly
  against the router at **192.168.1.1**. Both env overlays target the
  host NIC `enp45s0` (RTL8125 2.5GbE); base carries the
  `__NODE_NIC__` placeholder until the per-env patch fills it.

## DHCP dependency

Secondary interfaces get their addresses from the LAN router's DHCP server
(192.168.1.1) — no IPAM is configured in-cluster. If guests fail to get an
address, check the router's DHCP pool/scope before suspecting Multus.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | DaemonSet per-node (N/A) | NAD master → `enp45s0` |
| `prd` | DaemonSet per-node (N/A) | NAD master → `enp45s0` |

Multus is a thick-plugin DaemonSet — one pod per node by design, no
replica concept. Controllers track `../base` with no patches in both
envs.

Upstream reference (read-only): `/tmp/home-ops-docs/multus-docs`.

## Telemetry-off / monitoring / updates

- No reporting knobs exist upstream (DaemonSet only) — nothing to disable.
  Multus exposes metrics via plain prometheus annotations;
  ServiceMonitors wait for the monitoring stack (same discipline as §9).
- Version bumps via `update-policies/multus.yaml` (`>=4.3.0`) → PR automation;
  the `$imagepolicy` marker lives in the header of
  `controllers/base/multus-daemonset.yaml` (re-download runbook above).
