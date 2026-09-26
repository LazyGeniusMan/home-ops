# Reloader

Reloader chart 2.2.17 (app v1.4.22,
`oci://chartproxy.container-registry.com/stakater.github.io/stakater-charts/reloader`,
proxy of the classic-only `https://stakater.github.io/stakater-charts`): restarts
workloads when ESO-synced Secrets rotate. Canonical auto-reload for every
tenant — other slices opt in with `reloader.stakater.com/auto: "true"` on the
workload that consumes the rotating Secret.

## How it works

`reloader.watchGlobally: true` (single global ClusterRole). `reloadStrategy:
annotations` — Reloader patches pod-template annotations, which Flux v2 SSA
does not own (env-vars injection would be reverted on the next reconcile).
`autoReloadAll: false`; `ignoreSecrets/ignoreConfigMaps/ignoreJobs/ignoreCronJobs`
all false. `enableHA: false` (single replica unless HA is wanted).

## Annotating workloads

```yaml
metadata:
  annotations:
    reloader.stakater.com/auto: "true"
```

Annotated consumers in this scope: Zitadel Deployments (`zitadel`,
`zitadel-login`), ESO webhook Deployments, Zitadel login-proxy Terraform runner
(via its `varsFrom` Secret), Dragonfly operator Deployment. Dragonfly `Dragonfly`
CR pods are operator-owned — annotate the operator Deployment only; cache
password rotation takes effect on pod restart via the operator StatefulSet
reconcile.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | HPA 1-2 (seed 1) | seed -> 1, HPA 1/2, podMonitor `otel-scrape: "true"` |
| `prd` | HPA 2-4 (seed 2) | seed -> 2, HPA 2/4, podMonitor `otel-scrape: "true"` |

Controllers inherit `../base` unchanged otherwise. Configs shells are empty
(`resources: []`) — no component config ships here.

## Telemetry / monitoring / updates

No phone-home knobs in chart values. `podMonitor.enabled: true` (ServiceMonitor
deprecated upstream); monitoring CRDs arrive via the otel-operator infra-crds
delivery (prune:false), and the gateway TA scrapes monitors labeled
`otel-scrape: "true"`. Bumps: `update-policies/reloader.yaml` (>=2.2.17, marker
`infra:reloader:tag`) -> PR automation. Verify with `helm show chart` against
the chartproxy URL before pinning (upstream is classic-only).
Changelog: https://github.com/stakater/Reloader/releases.
