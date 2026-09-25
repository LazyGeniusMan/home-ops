# Cilium

Cilium 1.20.2 (`oci://quay.io/cilium/charts/cilium`): eBPF dataplane,
kube-proxy replacement, Gateway API support, Hubble observability, single-IP
`LoadBalancer` pool.

Minor => check the Gateway required version first: 1.20 requires Gateway
API v1.6.1 (`gateway-api/crds/base/standard-install.yaml`). Upgrade Gateway
CRDs before the Cilium chart. `upgradeCompatibility: "1.20"` pins datapath
behaviour to the install minor; bump only per the Cilium upgrade guide.

Preflight is a one-off imperative check -- never leave `preflight.enabled`
in values. Render with `preflight.enabled=true, agent=false,
operator.enabled=false` plus the per-env Talos API VIP, verify the
pre-flight DaemonSet Ready, then delete it before landing the chart bump.

## Values

- `kubeProxyReplacement: true` (Talos runs kube-proxy disabled).
- `k8sServiceHost`/`k8sServicePort`: Talos K8s API VIP (placeholder
  `__TALOS_API_VIP__`, 6443) -- not the LB VIP.
- `gatewayAPI.enabled: true`; Hubble relay on with prometheus metrics;
  all four `ServiceMonitor`s on (hubble relay/metrics, operator, agent —
  agent endpoint :9962 hostPort per node) with the `otel-scrape: "true"`
  label via the dev/prd overlay patches; `kube-system` carries the same
  label from `controllers/base/kube-system.yaml` (prune-disabled holder
  for the pre-existing Talos namespace).
- Single release identity with the Terraform bootstrap Job: release
  `cilium` in `kube-system` (`targetNamespace` + `storageNamespace`).

## VIP split

| IP | Role |
| --- | --- |
| 192.168.1.198 | Talos K8s API VIP (Layer2VIP, Talos-owned) |
| 192.168.1.199 | `CiliumLoadBalancerIPPool/default` LB VIP |

Pool covers only `.199`, never `.198`. `CiliumL2AnnouncementPolicy`
announces LB IPs on Talos NIC `enp45s0`.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | DaemonSet per-node + operator 1 | K8s API VIP `.248`, LB pool `.249`, NIC `ens18` |
| `prd` | DaemonSet per-node + operator 2 | K8s API VIP `.198`, LB pool `.199`, NIC `enp45s0` |

## Telemetry / monitoring / updates

Chart has no usage-reporting keys. All four `serviceMonitor.enabled: true`.
Chart bumps: `update-policies/cilium.yaml` (>=1.20.2, marker
`infra:cilium:tag`) -> PR automation. Ships in the initial tenant wave
before workloads.
Changelog: https://github.com/cilium/cilium/releases.
