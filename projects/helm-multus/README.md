# helm-multus

Multus thick-plugin DaemonSet as a Helm OCI chart. This chart commits **no YAML** — `ci/fetch.sh` downloads the
upstream release asset(s) at publish time into the gitignored `upstream/`
staging dir, and `helm package` bundles them. Flux consumes the published OCI
artifact (`oci://ghcr.io/lazygeniusman/home-ops/projects/helm-multus`).

The fetch script re-applies two local narrowings:

1. **ClusterRole narrowing** — NAD get/list/watch + pods get/list/watch/update + pods/status get/update/patch + events create/patch (upstream grants `k8s.cni.cncf.io:*` on `*`).
2. **Image re-pin** — both image fields re-pin to `ghcr.io/k8snetworkplumbingwg/multus-cni:<tag>-thick` (upstream ships `snapshot-thick` placeholders).

Thick mode serves KubeVirt secondary-net. Upstream shape changes fail the fetch.

Upstream source: https://github.com/k8snetworkplumbingwg/multus-cni/releases
(asset `deployments/multus-daemonset-thick.yml` at tag `v<VERSION>`).

## Version contract

`Chart.yaml` `version` == upstream release sans leading `v`
(`4.3.0` -> tag `v4.3.0`); `appVersion` is the same tag with the `v`.
The check workflow bumps both together. No ImagePolicy: CRD releases bump by hand.

## Fetch + publish flow

```bash
bash projects/helm-multus/ci/fetch.sh --check   # URL(s) answer 200 for Chart.yaml version
bash projects/helm-multus/ci/fetch.sh           # stage into upstream/
helm package projects/helm-multus               # bundles staged upstream/
helm push helm-multus-<version>.tgz oci://ghcr.io/lazygeniusman/home-ops/projects
bash projects/helm-multus/ci/verify.sh          # lint + fetch dry-run + no-CRD-committed guard
```

Push-only publish from the `Chart.yaml` version (no git tags); re-publishing overwrites the same OCI tag.
