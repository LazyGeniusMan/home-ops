# Cilium (§8.2)

Cilium v1.20.1: eBPF dataplane, kube-proxy replacement, Gateway API support,
Hubble observability, and a single-IP `LoadBalancer` pool.

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
- Agent `prometheus.enabled: false` — agent metrics off for now (plain
  `if enabled` gates, cheap to flip later).

## VIP split

| IP            | Role                                              |
| ------------- | ------------------------------------------------- |
| 192.168.1.198 | Talos K8s API VIP (Layer2VIP, owned by Talos)     |
| 192.168.1.199 | Cilium `CiliumLoadBalancerIPPool/default` (LB VIP)|

The pool covers only `.199` — never `.198`. `CiliumL2AnnouncementPolicy`
announces LB IPs on the Talos NIC `enp45s0`.

## ServiceMonitor deviation (§9)

All `serviceMonitor.enabled: false` mirrors the cert-manager deviation:
plain `prometheus.enabled` only adds scrape annotations (safe without a
stack), while `ServiceMonitor` objects require the monitoring.coreos.com
CRDs. Flip them on once the monitoring stack lands.

## Telemetry-off evidence

`helm show values oci://quay.io/cilium/charts/cilium --version 1.20.1 |
grep -viE '^\s*#' | grep -iE 'telemetry|usageReporting|phoneHome|analytics'`
returns empty — the chart has no usage-reporting *keys* (only comment
mentions such as "Disable the usage of CiliumEndpoint CRD"), so there is
nothing to switch off.

## Update automation

`flux/infra/update-policies/cilium.yaml` (`ImageRepository` +
`ImagePolicy`, semver `>=1.20.1`) tracks `quay.io/cilium/charts/cilium`;
ImageUpdateAutomation opens PRs via the `$imagepolicy` marker on the
`OCIRepository` tag.

## Tenant wave

Cilium ships in the initial tenant wave (`dependsOn` policies) before
workloads — pods need the CNI + LB pool before anything schedules.
