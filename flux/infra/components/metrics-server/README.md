# metrics-server (§9.2)

Metrics Server app v0.9.0 (chart 3.14.0) serving the `v1beta1.metrics.k8s.io`
API (`kubectl top`, HPA/VPA).

## Telemetry-off / monitoring / updates

Upstream chart exposes no reporting knobs; values set only `apiService`,
`metrics`, `serviceMonitor`.

- `/metrics` exposed (`metrics.enabled`); `ServiceMonitor: off`.
- Chart bumps: `update-policies/metrics-server.yaml` → PR automation.

## Upgrade runbook

- Version source: the `OCIRepository` tag in
  `controllers/base/metrics-server.yaml` (chart 3.14.0, app v0.9.0).
- Changelog: https://github.com/kubernetes-sigs/metrics-server/releases.
- Bump: let the ImagePolicy PR land (marker `infra:metrics-server:tag`,
  `update-policies/metrics-server.yaml`). Risk is low (stateless
  scraper, no stored state).
- Verify: Deployment `Ready`, then `kubectl top nodes` and
  `kubectl top pods -A` return values within a minute.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | HPA 1–2; replica seed 1 | `controllers/dev` pins seed → 1 + HPA min 1 / max 2 |
| `prd` | HPA 2–4 (base values); replica seed 2 | `controllers/prd` pins seed → 2 |

Upstream reference (read-only): `/tmp/home-ops-docs/metrics-server-docs`.
