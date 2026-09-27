# Cilium

Cilium 1.20.2 (`oci://quay.io/cilium/charts/cilium`): eBPF dataplane,
kube-proxy replacement, Gateway API support, Hubble observability, single-IP
`LoadBalancer` pool.

Minor => check the Gateway required version first: 1.20 requires Gateway
API v1.6.1 (`gateway-api/crds/base/standard-install.yaml`, first-party
`helm-gateway-api` chart). Upgrade the Gateway chart before the Cilium chart.
`upgradeCompatibility: "1.20"` pins datapath
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
- `encryption.enabled: true` + `type: wireguard` (automatic per-node keys;
  `nodeEncryption: false` — pod-to-pod encrypted, host traffic plain on the
  trusted LAN). Trust boundary: the cluster runs private on the trusted LAN
  (no public node ingress; user traffic enters only through the NetBird mesh
  and the Gateway). Service-domain cleartext is CORRECT — every cleartext
  service-domain `http://` endpoint in the repo (S3 :8333, OTLP :4317/:4318,
  ESO webhook :8080, Dragonfly :6379, ClickHouse :8123/:9000) stays
  unencrypted by design with the CNI owning wire encryption; edge TLS
  terminates ONLY at the Gateway API + cert-manager layer, never in-cluster.
  Single-node veth/host cleartext is accepted (same-node pods share the host
  path with no WireGuard hop — no secret traverses it that is not already
  visible to the host root).

## VIP split

| IP | Role |
| --- | --- |
| 192.168.1.198 | Talos K8s API VIP (Layer2VIP, Talos-owned) |
| 192.168.1.199 | `CiliumLoadBalancerIPPool/default` LB VIP |

Pool covers only `.199`, never `.198`. `CiliumL2AnnouncementPolicy`
announces LB IPs on the Talos node NIC (`enp45s0` prd, `ens18` dev).

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
