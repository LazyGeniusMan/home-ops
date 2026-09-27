# Cilium

Cilium 1.20.2 (`oci://quay.io/cilium/charts/cilium`): eBPF dataplane, kube-proxy replacement, Gateway API support, Hubble observability, single-IP `LoadBalancer` pool. In-cluster service traffic stays cleartext by design — Cilium WireGuard (`encryption.type: wireguard`, node encryption off) owns wire encryption; TLS terminates only at the Gateway (see AGENTS.md encryption boundary).

Minor bumps: check the Gateway required version first (1.20 needs Gateway API v1.6.1; upgrade `helm-gateway-api` before Cilium). `upgradeCompatibility: "1.20"` pins datapath behaviour; preflight is a one-off imperative check, never left in values.

## Values

- `kubeProxyReplacement: true` (Talos runs kube-proxy disabled).
- `k8sServiceHost`/`k8sServicePort`: Talos K8s API VIP (placeholder `__TALOS_API_VIP__`, 6443), not the LB VIP.
- `gatewayAPI.enabled: true`; Hubble relay on with prometheus metrics; all four `ServiceMonitor`s on with the `otel-scrape: "true"` label via overlays (`kube-system` carries the same label from `controllers/base/kube-system.yaml`, prune-disabled holder for the pre-existing Talos namespace).
- Single release identity with the Terraform bootstrap Job: release `cilium` in `kube-system`.

## VIP split

| IP | Role |
| --- | --- |
| 192.168.1.198 | Talos K8s API VIP (Layer2VIP, Talos-owned) |
| 192.168.1.199 | `CiliumLoadBalancerIPPool/default` LB VIP |

Pool covers only `.199`, never `.198`. `CiliumL2AnnouncementPolicy` announces LB IPs on the Talos node NIC (`enp45s0` prd, `ens18` dev).

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | DaemonSet per-node + operator 1 | K8s API VIP `.248`, LB pool `.249`, NIC `ens18` |
| `prd` | DaemonSet per-node + operator 2 | K8s API VIP `.198`, LB pool `.199`, NIC `enp45s0` |

## Updates

`update-policies/cilium.yaml` (>=1.20.2, marker `infra:cilium:tag`). Ships in the initial tenant wave before workloads. Changelog: https://github.com/cilium/cilium/releases.
