# vpa (§9.x)

Vertical Pod Autoscaler Fairwinds chart 5.0.1 (app v1.7.1 — current default
per `vpa-docs/vertical-pod-autoscaler/docs/installation.md`) serving resource
recommendations (`kubectl describe vpa`) for every workload; applying
(`Recreate`) only on singletons, recommender-only (`Off`) where HPA owns CPU.

## Telemetry-off evidence

Upstream chart exposes no reporting knobs; values set only `recommender`,
`updater`, `admissionController` (certgen + resources).

## Monitoring / updates

- No `ServiceMonitor`/`podMonitor` keys enabled (both default `false`);
  flip per component when `monitoring.coreos.com` CRDs land.
- Chart bumps: `update-policies/vpa.yaml` → PR automation.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (singleton) | confirms `recommender.replicaCount: 1` |
| `prd` | 1 (singleton) | confirms `recommender.replicaCount: 1` |

Upstream reference (read-only): `/tmp/home-ops-docs/vpa-docs`.
