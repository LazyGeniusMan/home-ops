# metrics-server

Metrics Server app v0.9.0 (chart 3.14.0,
`oci://ghcr.io/controlplaneio-fluxcd/charts/metrics-server`) serving the
`v1beta1.metrics.k8s.io` API (`kubectl top`, HPA/VPA).

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | HPA 1-2; replica seed 1 |
| `prd` | HPA 2-4 (base values); replica seed 2 |

## Telemetry / monitoring / updates

Upstream chart exposes no reporting knobs; values set only `apiService`,
`metrics`, `serviceMonitor`. `/metrics` exposed (`metrics.enabled`);
`ServiceMonitor` off. Bumps: `update-policies/metrics-server.yaml`
(>=3.14.0, marker `infra:metrics-server:tag`) -> PR automation.
Changelog: https://github.com/kubernetes-sigs/metrics-server/releases.
