# metrics-server

Metrics Server app v0.9.0 (chart 3.14.0, `oci://ghcr.io/controlplaneio-fluxcd/charts/metrics-server`) serving the `v1beta1.metrics.k8s.io` API (`kubectl top`, HPA/VPA). No HPA/VPA on itself by design (it feeds the Metrics API every sibling HPA consumes, so self-scaling would be circular).

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | fixed 1 (no HPA) |
| `prd` | fixed 2 (no HPA) |

## Updates

`update-policies/metrics-server.yaml` (>=3.14.0, marker `infra:metrics-server:tag`). Changelog: https://github.com/kubernetes-sigs/metrics-server/releases.
