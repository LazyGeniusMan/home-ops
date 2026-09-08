# multus (§13.1)

Multus v4.3.0 thick-plugin DaemonSet (`controllers/base/multus-daemonset.yaml`)
plus the `lan-dhcp` NetworkAttachmentDefinition (`configs/base/lan-dhcp.yaml`).

## Source: vendored, not charted

No official upstream chart exists — the bounded OCI check found only
third-party charts (Bitnami, TrueCharts) — so Gitless = vendored pinned
release manifest + flux-pushed OCI artifact:

- Upstream:
  `https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/v4.3.0/deployments/multus-daemonset-thick.yml`
  (v4.3.0 verified: real GitHub release, not draft/prerelease).
- Image pin: upstream tags the DaemonSet `snapshot-thick` (floating); both
  image fields (daemon + install-multus-binary init container) are pinned to
  `ghcr.io/k8snetworkplumbingwg/multus-cni:v4.3.0-thick` (verified the tag
  exists on GHCR via the API, all 200s).
- The `kube-system` ServiceAccount/ClusterRole(Binding)/ConfigMap/DaemonSet
  namespaces are upstream's and are correct as-is: infra-controllers applies
  with targetNamespace=multus but cluster-scoped resources (CRD, ClusterRole,
  ClusterRoleBinding) are untouched by it, and the multus namespace is created
  by the tenant entry.

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
  against the router at **192.168.1.1**. Base master targets the prd host NIC
  (`enp45s0` = RTL8125 2.5GbE); the stg overlay repatches master to the
  dev virtio NIC (`eth0`, QEMU/KVM).

## DHCP dependency

Secondary interfaces get their addresses from the LAN router's DHCP server
(192.168.1.1) — no IPAM is configured in-cluster. If guests fail to get an
address, check the router's DHCP pool/scope before suspecting Multus.

## Environments

`prd` and `stg` controllers track `../base` with no patches;
stg configs repatch the NAD master to `eth0` (above).

## Telemetry-off / monitoring / updates

- No reporting knobs exist upstream (DaemonSet only) — nothing to disable.
  Multus exposes metrics via plain prometheus annotations;
  ServiceMonitors wait for the monitoring stack (same discipline as §9).
- Version bumps via `update-policies/multus.yaml` (`>=4.3.0`) → PR automation;
  the `$imagepolicy` marker lives in the header of
  `controllers/base/multus-daemonset.yaml`. Update procedure: re-download the
  upstream URL at the new tag, diff against the vendored file (re-apply the
  image pin + header), then bump the marker.
