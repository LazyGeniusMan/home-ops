# helm-kubevirt

KubeVirt operator bundle as a Helm OCI chart. This chart commits **no YAML** — `ci/fetch.sh` downloads the
upstream release asset(s) at publish time into the gitignored `upstream/`
staging dir, and `helm package` bundles them. Flux consumes the published OCI
artifact (`oci://ghcr.io/lazygeniusman/home-ops/projects/helm-kubevirt`).

Single chart containing the operator bundle only (operator Deployment + RBAC + the `KubeVirt` CRD). The `KubeVirt` custom resource stays a Flux-managed manifest in `flux/infra/components/kubevirt/configs/base/kubevirt-cr.yaml`.

Upstream source: https://github.com/kubevirt/kubevirt/releases
(asset `kubevirt-operator.yaml` at
`.../releases/download/v<VERSION>/kubevirt-operator.yaml`; upgrades N-1 -> N
only, operator-first).

## Version contract

`Chart.yaml` `version` == upstream release sans leading `v`
(`1.9.0` -> tag `v1.9.0`); `appVersion` is the same tag with the `v`.
The check workflow bumps both together. No ImagePolicy: CRD releases bump by hand.

## Fetch + publish flow

```bash
bash projects/helm-kubevirt/ci/fetch.sh --check   # URL(s) answer 200 for Chart.yaml version
bash projects/helm-kubevirt/ci/fetch.sh           # stage into upstream/
helm package projects/helm-kubevirt               # bundles staged upstream/
helm push helm-kubevirt-<version>.tgz oci://ghcr.io/lazygeniusman/home-ops/projects
bash projects/helm-kubevirt/ci/verify.sh          # lint + fetch dry-run + no-CRD-committed guard
```

Push-only publish from the `Chart.yaml` version (no git tags); re-publishing overwrites the same OCI tag.
