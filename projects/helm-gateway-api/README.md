# helm-gateway-api

Gateway API standard-channel CRDs as a Helm OCI chart. This chart commits **no
CRD YAML** — `ci/fetch.sh` downloads the upstream release asset at publish
time into the gitignored `upstream/` staging dir, and `helm package` bundles
it. Flux consumes the published OCI artifact
(`oci://ghcr.io/lazygeniusman/home-ops/projects/helm-gateway-api`).

Upstream source: https://github.com/kubernetes-sigs/gateway-api/releases
(asset `standard-install.yaml` at
`.../releases/download/v<VERSION>/standard-install.yaml`; standard channel only).

## Version contract

`Chart.yaml` `version` == upstream release sans leading `v`
(`1.6.1` -> tag `v1.6.1`); `appVersion` is the same tag with the `v`.
The check workflow bumps both together. No ImagePolicy: CRD releases bump by hand.

## Fetch + publish flow

```bash
bash projects/helm-gateway-api/ci/fetch.sh --check   # URL answers 200 for Chart.yaml version
bash projects/helm-gateway-api/ci/fetch.sh           # stage into upstream/
helm package projects/helm-gateway-api               # bundles staged upstream/
helm push helm-gateway-api-<version>.tgz oci://ghcr.io/lazygeniusman/home-ops/projects
bash projects/helm-gateway-api/ci/verify.sh          # lint + fetch dry-run + no-CRD-committed guard
```

Push-only publish from the `Chart.yaml` version (no git tags); re-publishing overwrites the same OCI tag.

<!-- trigger CI -->
