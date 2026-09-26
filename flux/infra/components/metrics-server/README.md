# metrics-server

Metrics Server app v0.9.0 (chart 3.14.0,
`oci://ghcr.io/controlplaneio-fluxcd/charts/metrics-server`) serving the
`v1beta1.metrics.k8s.io` API (`kubectl top`, HPA/VPA).

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | fixed 1 (no HPA) |
| `prd` | fixed 2 (no HPA) |

No HPA/VPA by design: metrics-server feeds the Metrics API every sibling HPA
consumes, so an HPA on itself is circular (a Metrics API outage would freeze
its own scaling).

## Telemetry / monitoring / updates

Upstream chart exposes no reporting knobs; values set only `apiService`,
`metrics`, `serviceMonitor`. `/metrics` exposed (`metrics.enabled`);
`ServiceMonitor` on (monitoring CRDs via the infra-crds tenant). Bumps: `update-policies/metrics-server.yaml`
(>=3.14.0, marker `infra:metrics-server:tag`) -> PR automation.
Changelog: https://github.com/kubernetes-sigs/metrics-server/releases.
