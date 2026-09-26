# helm-otel-monitoring-crds

Prometheus-operator monitoring CRDs (ServiceMonitor + PodMonitor only) as a Helm OCI chart. This chart commits **no YAML** — `ci/fetch.sh` downloads the
upstream release asset(s) at publish time into the gitignored `upstream/`
staging dir, and `helm package` bundles them. Flux consumes the published OCI
artifact (`oci://ghcr.io/lazygeniusman/home-ops/projects/helm-otel-monitoring-crds`).

Monitoring scope ONLY — ServiceMonitor + PodMonitor, no Prometheus/PrometheusRule/Alertmanager/Grafana — so sibling charts can flip their `*_monitor` knobs to `skipIfMissing`.

Upstream source: https://github.com/prometheus-operator/prometheus-operator/tree/v<VERSION>/example/prometheus-operator-crd
(files `monitoring.coreos.com_servicemonitors.yaml` + `monitoring.coreos.com_podmonitors.yaml`).

## Version contract

`Chart.yaml` `version` == upstream release sans leading `v`
(`0.93.1` -> tag `v0.93.1`); `appVersion` is the same tag with the `v`.
The check workflow bumps both together; there is deliberately **no ImagePolicy**
— image automation only tracks images/charts, and a floating policy over CRD
releases would auto-propose API-surface changes without a human re-vendoring
the narrowing/migration notes beside the code.

## Fetch + publish flow

```bash
bash projects/helm-otel-monitoring-crds/ci/fetch.sh --check   # URL(s) answer 200 for Chart.yaml version
bash projects/helm-otel-monitoring-crds/ci/fetch.sh           # stage into upstream/
helm package projects/helm-otel-monitoring-crds               # bundles staged upstream/
helm push helm-otel-monitoring-crds-<version>.tgz oci://ghcr.io/lazygeniusman/home-ops/projects
bash projects/helm-otel-monitoring-crds/ci/verify.sh          # lint + fetch dry-run + no-CRD-committed guard
```

Tag releases as `helm-otel-monitoring-crds-v<semver>` (see `scripts/tag-release.sh`);
the publish workflow runs `ci/fetch.sh` before `helm package`, signs with
cosign, and moves the `stable`/`dev` floating tags.
