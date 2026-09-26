# helm-gateway-api

Gateway API standard-channel CRDs as a Helm OCI chart. This chart commits **no
CRD YAML** — `ci/fetch.sh` downloads the upstream release asset at publish
time into the gitignored `upstream/` staging dir, and `helm package` bundles
it. Flux consumes the published OCI artifact
(`oci://ghcr.io/lazygeniusman/home-ops/projects/helm-gateway-api`).

Upstream source: https://github.com/kubernetes-sigs/gateway-api/releases
(asset `standard-install.yaml` at
`.../releases/download/v<VERSION>/standard-install.yaml`; standard channel:
10 Gateway CRDs + the safe-upgrades ValidatingAdmissionPolicy/Binding —
experimental channel NOT adopted).

## Version contract

`Chart.yaml` `version` == upstream release sans leading `v`
(`1.6.1` -> tag `v1.6.1`); `appVersion` is the same tag with the `v`.
The check workflow bumps both together; there is deliberately **no ImagePolicy**
— image automation only tracks images/charts, and a floating policy over CRD
releases would auto-propose API-surface changes without a human re-vendoring
the narrowing/migration notes beside the code.

## Fetch + publish flow

```bash
bash projects/helm-gateway-api/ci/fetch.sh --check   # URL answers 200 for Chart.yaml version
bash projects/helm-gateway-api/ci/fetch.sh           # stage into upstream/
helm package projects/helm-gateway-api               # bundles staged upstream/
helm push helm-gateway-api-<version>.tgz oci://ghcr.io/lazygeniusman/home-ops/projects
bash projects/helm-gateway-api/ci/verify.sh          # lint + fetch dry-run + no-CRD-committed guard
```

Publish is push-only (no git tags): the publish workflow resolves the version
purely from `Chart.yaml`, signs with cosign, and moves the `stable`/`dev`
floating tags. Re-publishing an unchanged `Chart.yaml` version overwrites
the same OCI tag (idempotent).
