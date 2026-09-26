# helm-cosi

COSI objectstorage CRDs (5 files) as a Helm OCI chart. This chart commits **no YAML** — `ci/fetch.sh` downloads the
upstream release asset(s) at publish time into the gitignored `upstream/`
staging dir, and `helm package` bundles them. Flux consumes the published OCI
artifact (`oci://ghcr.io/lazygeniusman/home-ops/projects/helm-cosi`).

All 5 files re-vendor together from the tag (`objectstorage.k8s.io_bucketaccessclasses/bucketaccesses/bucketclaims/bucketclasses/buckets.yaml` under `client/config/crd`). Pins stay on the v0.2.x line; do NOT track main (v1alpha2, incompatible with the SeaweedFS driver).

Upstream source: https://github.com/kubernetes-sigs/container-object-storage-interface/tree/v<VERSION>/client/config/crd.

## Version contract

`Chart.yaml` `version` == upstream release sans leading `v`
(`0.2.2` -> tag `v0.2.2`); `appVersion` is the same tag with the `v`.
The check workflow bumps both together; there is deliberately **no ImagePolicy**
— image automation only tracks images/charts, and a floating policy over CRD
releases would auto-propose API-surface changes without a human re-vendoring
the narrowing/migration notes beside the code.

## Fetch + publish flow

```bash
bash projects/helm-cosi/ci/fetch.sh --check   # URL(s) answer 200 for Chart.yaml version
bash projects/helm-cosi/ci/fetch.sh           # stage into upstream/
helm package projects/helm-cosi               # bundles staged upstream/
helm push helm-cosi-<version>.tgz oci://ghcr.io/lazygeniusman/home-ops/projects
bash projects/helm-cosi/ci/verify.sh          # lint + fetch dry-run + no-CRD-committed guard
```

Publish is push-only (no git tags): the publish workflow resolves the version
purely from `Chart.yaml`, runs `ci/fetch.sh` before `helm package`, signs with
cosign, and moves the `stable`/`dev` floating tags. Re-publishing an unchanged
`Chart.yaml` version overwrites the same OCI tag (idempotent).
