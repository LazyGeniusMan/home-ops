# otel-operator

OTel Operator chart 0.123.1 (app 0.159.0, `oci://chartproxy.container-registry.com/open-telemetry.github.io/opentelemetry-helm-charts/opentelemetry-operator`) reconciling `OpenTelemetryCollector` CRs; webhooks via cert-manager. Own `ServiceMonitor` off (no Prometheus server in this repo).

`crds/base` ships ServiceMonitor + PodMonitor (prometheus-operator v0.93.1, monitoring scope only) via the first-party `helm-otel-monitoring-crds` OCI chart so sibling charts can flip their `*_monitor` knobs to `skipIfMissing`; renders through the fleet's prune:false infra-crds Kustomization.

## Environments

| Env | Replicas |
| --- | --- |
| `dev` | HPA 1-2; replica seed 1 |
| `prd` | HPA 2-4 (base values); replica seed 2 |

## Updates

Operator bumps via `update-policies/otel-operator.yaml` (>=0.123.1 <0.124.0, marker `infra:otel-operator:tag`); monitoring CRDs bump via Chart.yaml + wrapper `ref.tag` together (no ImagePolicy/marker, atomic hand-bump), human merges. Changelog: https://github.com/open-telemetry/opentelemetry-helm-charts/releases.
