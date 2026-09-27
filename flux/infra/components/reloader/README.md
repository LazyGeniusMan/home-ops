# Reloader

Reloader chart 2.2.17 (app v1.4.22, `oci://chartproxy.container-registry.com/stakater.github.io/stakater-charts/reloader`): restarts workloads when ESO-synced Secrets rotate. Canonical auto-reload for every tenant — other slices opt in with `reloader.stakater.com/auto: "true"` on the workload consuming the rotating Secret.

`reloader.watchGlobally: true` (single global ClusterRole); `reloadStrategy: annotations` (env-vars injection would be reverted by Flux SSA); `autoReloadAll: false`. Annotated: Zitadel Deployments, ESO webhook, external-dns, seaweedfs `ui-auth` proxy, Dragonfly operator (CR pods roll via the operator reconcile, so annotate the operator only). Exempt by design: Terraform `varsFrom` Secrets (runners re-read vars every 30m reconcile), COSI-minted S3 keys (rotate by re-minting the BucketAccess), otel-gateway `otel-clickhouse` mirror (operator-owned CRs, not Deployments).

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | HPA 1-2 (seed 1) | seed -> 1, HPA 1/2, podMonitor `otel-scrape: "true"` |
| `prd` | HPA 2-4 (seed 2) | seed -> 2, HPA 2/4, podMonitor `otel-scrape: "true"` |

Configs shells are empty (`resources: []`); the HPA-scaled Deployment carries a PDB (`minAvailable: 1`).

## Updates

`update-policies/reloader.yaml` (>=2.2.17, marker `infra:reloader:tag`). Changelog: https://github.com/stakater/Reloader/releases.
