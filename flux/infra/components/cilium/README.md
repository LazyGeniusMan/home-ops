# Cilium (§8.2)

Cilium v1.20.2: eBPF dataplane, kube-proxy replacement, Gateway API support,
Hubble observability, and a single-IP `LoadBalancer` pool.

## Minor-upgrade pairing invariant

Cilium minor => check the Gateway required version first: every Cilium
minor pins a minimum Gateway API bundle (1.20 requires v1.6.1 — see
`gateway-api/crds/base/standard-install.yaml`, currently v1.6.1).
Upgrade the Gateway API CRDs **before** the Cilium chart, then bump
`upgradeCompatibility` only deliberately per the Cilium upgrade guide
(it pins datapath behaviour to the initial install minor; currently
`"1.20"`).

## Preflight runbook (manual, per minor upgrade)

Preflight is a one-off imperative check — never leave
`preflight.enabled` in the release values. Render with
`preflight.enabled=true, agent=false, operator.enabled=false` plus the
per-env Talos API VIP (`k8sServiceHost`/`k8sServicePort`), create the
rendered manifest, verify the pre-flight DaemonSet/check report Ready,
then delete it. Only then let the ImagePolicy PR land.

## Values rationale

- `kubeProxyReplacement: true` — Talos runs with kube-proxy disabled, so
  Cilium takes over ClusterIP/NodePort handling (strict-mode replacement).
- `k8sServiceHost: 192.168.1.198` / `k8sServicePort: 6443` — the Talos K8s
  API VIP, so agents reach the API server without kube-proxy.
- `gatewayAPI.enabled: true` — §8 routes ingress through Cilium Gateway API.
- `hubble.relay.enabled: true` + `hubble.relay.prometheus.enabled: true` —
  Hubble UI/CLI access with metrics; `ServiceMonitor` off (see below).
- `hubble.metrics.enabled: [dns drop tcp flow icmp http]` — flow metrics on.
- `operator.prometheus.enabled: true` — operator metrics endpoint on.
- Agent `prometheus.enabled: false` — agent metrics off.

## VIP split

| IP            | Role                                              |
| ------------- | ------------------------------------------------- |
| 192.168.1.198 | Talos K8s API VIP (Layer2VIP, owned by Talos)     |
| 192.168.1.199 | Cilium `CiliumLoadBalancerIPPool/default` (LB VIP)|

The pool covers only `.199` — never `.198`. `CiliumL2AnnouncementPolicy`
announces LB IPs on the Talos NIC `enp45s0`.

## Telemetry-off / monitoring / updates

- The chart has no usage-reporting keys, so there is nothing to switch off.
- All `serviceMonitor.enabled: false`: plain `prometheus.enabled` only adds
  scrape annotations, while `ServiceMonitor` objects require the
  monitoring.coreos.com CRDs.
- Chart bumps: `update-policies/cilium.yaml` → PR automation.
- Cilium ships in the initial tenant wave (`dependsOn` policies) before
  workloads — pods need the CNI + LB pool before anything schedules.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | DaemonSet per-node (N/A) + operator 1 (pinned in `controllers/dev`) | Talos K8s API VIP `192.168.1.248`, LB pool `.249`, node NIC `ens18` |
| `prd` | DaemonSet per-node (N/A) + operator 2 (pinned in `controllers/prd`) | Talos K8s API VIP `192.168.1.198`, LB pool `.199`, node NIC `enp45s0` |

The agent is a DaemonSet — one pod per node by design, no replica
concept. Operator `replicas` pins to 1 in `controllers/dev`, 2 in
`controllers/prd`. Hostnames/VIPs ride the per-env controller + configs
patches above.

Upstream reference (read-only): `/tmp/home-ops-docs/cilium-docs`.
