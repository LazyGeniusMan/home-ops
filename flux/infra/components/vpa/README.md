# vpa (§9.x)

Vertical Pod Autoscaler official upstream chart `vertical-pod-autoscaler`
0.12.0 (app v1.7.1 — current default per
`vpa-docs/vertical-pod-autoscaler/docs/installation.md`; same app as the
retired Fairwinds `vpa` 5.0.1, chart-only migration) via classic
`HelmRepository` `https://kubernetes.github.io/autoscaler`. Serving resource
recommendations (`kubectl describe vpa`) for every workload; applying
(`Recreate`) only on singletons, recommender-only (`Off`) where HPA owns CPU.

## Values keys (upstream shape)

- `recommender` / `updater` / `admissionController` each take `replicas`
  (not `replicaCount`), plus `logLevel` + `extraArgs` (list, not map).
- `admissionController`: Helm-managed certgen (`registerWebhook: false` +
  `certGen.enabled: true`); cert-manager path skipped (mutually exclusive
  with certGen, adds Issuer/Certificate CRs that wedge bootstrap ordering —
  see `controllers/base/vpa.yaml` header).
- `crds.enabled/keep: true` (CRDs render as regular templates; Flux replaces
  via `crds: CreateReplace`).
- Stack PDBs default `minAvailable: 1`; updater binary default
  `--min-replicas=2` stands, so singleton targets need
  `updatePolicy.minReplicas: 1` on their VPA CRs to ever evict.

## Telemetry-off evidence

Upstream chart exposes no reporting knobs; values set only `recommender`,
`updater`, `admissionController` (certgen + resources).

## Monitoring / updates

- No `ServiceMonitor` keys exist in the chart (verified: no matches in
  `values.yaml`); flip per component when `monitoring.coreos.com` CRDs land.
- Chart bumps: `update-policies/vpa.yaml` → PR automation (nominal feed —
  chart bumps stay manual via the `$imagepolicy` marker).

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (singleton) | confirms `recommender.replicas: 1` |
| `prd` | 1 (singleton) | confirms `recommender.replicas: 1` |

Upstream reference (read-only): `/tmp/home-ops-docs/vpa-docs`.
