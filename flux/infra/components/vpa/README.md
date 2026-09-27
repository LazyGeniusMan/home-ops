# vpa

Vertical Pod Autoscaler chart 0.12.0 (app v1.7.1, `oci://chartproxy.container-registry.com/kubernetes.github.io/autoscaler/vertical-pod-autoscaler`): serving resource recommendations (`kubectl describe vpa`); applying (`Recreate`) only on singletons, recommender-only (`Off`) where HPA owns CPU.

`recommender` / `updater` / `admissionController` each take `replicas` (not `replicaCount`) plus `logLevel` + `extraArgs` (list, not map). `admissionController`: Helm-managed certgen (`registerWebhook: false` + `certGen.enabled: true`); `crds.enabled/keep: true`. Stack PDBs default `minAvailable: 1`; updater binary default `--min-replicas=2` stands, so singleton targets need `updatePolicy.minReplicas: 1` on their VPA CRs to ever evict.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | 1 (singleton) |
| `prd` | 1 (singleton) |

## Updates

Manual bumps: the update policy polls the recommender app image while the range pins the chart line (`>=0.12.0 <0.13.0`), so no automation PR arrives — move the `semver` ref by hand with the `$imagepolicy` marker (`infra:vpa:tag`) and keep the range on the new chart line. Changelog: https://github.com/kubernetes/autoscaler/releases.
