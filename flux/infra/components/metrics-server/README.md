# metrics-server (§9.2)

Metrics Server app v0.9.0 (chart 3.14.0) serving the `v1beta1.metrics.k8s.io`
API (`kubectl top`, HPA/VPA).

## Telemetry-off evidence

Upstream chart exposes no reporting knobs; values set only `apiService`,
`metrics`, `serviceMonitor`.

## Monitoring / updates

- `/metrics` exposed (`metrics.enabled`); `ServiceMonitor` stays disabled
  until `monitoring.coreos.com` CRDs land — flip
  `serviceMonitor.enabled` then (same values file).
- Chart bumps: `update-policies/metrics-server.yaml` → PR automation.
