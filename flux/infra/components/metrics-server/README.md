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

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (chart default; single-instance) | none — inherits `../base` unchanged |
| `prd` | 2 recommended (survive a node loss once multi-node) | none yet — scale the Deployment to 2 when the second node lands |

Upstream reference (read-only): `/tmp/home-ops-docs/metrics-server-docs`.
