# helm-multus

Multus thick-plugin DaemonSet as a Helm OCI chart. This chart commits **no YAML** — `ci/fetch.sh` downloads the
upstream release asset(s) at publish time into the gitignored `upstream/`
staging dir, and `helm package` bundles them. Flux consumes the published OCI
artifact (`oci://ghcr.io/lazygeniusman/home-ops/projects/helm-multus`).

The fetch script re-applies the two local narrowings as script steps so the published bundle matches the previously vendored file:

1. **ClusterRole narrowing** — upstream grants `k8s.cni.cncf.io:*` on verbs `*`; this repo keeps NAD get/list/watch + pods get/list/watch/update + pods/status get/update/patch + events create/patch.
2. **Image re-pin** — upstream ships `snapshot-thick` placeholders; both image fields (daemon + install-multus-binary init container) re-pin together to `ghcr.io/k8snetworkplumbingwg/multus-cni:<tag>-thick` (never `snapshot-thick`).

Thick mode serves KubeVirt secondary-net. If the upstream ClusterRole block changes shape, `ci/fetch.sh` fails loudly (`update the narrowing`) instead of shipping widened RBAC.

Upstream source: https://github.com/k8snetworkplumbingwg/multus-cni/releases
(asset `deployments/multus-daemonset-thick.yml` at tag `v<VERSION>`).

## Version contract

`Chart.yaml` `version` == upstream release sans leading `v`
(`4.3.0` -> tag `v4.3.0`); `appVersion` is the same tag with the `v`.
The check workflow bumps both together; there is deliberately **no ImagePolicy**
— image automation only tracks images/charts, and a floating policy over CRD
releases would auto-propose API-surface changes without a human re-vendoring
the narrowing/migration notes beside the code.

## Fetch + publish flow

```bash
bash projects/helm-multus/ci/fetch.sh --check   # URL(s) answer 200 for Chart.yaml version
bash projects/helm-multus/ci/fetch.sh           # stage into upstream/
helm package projects/helm-multus               # bundles staged upstream/
helm push helm-multus-<version>.tgz oci://ghcr.io/lazygeniusman/home-ops/projects
bash projects/helm-multus/ci/verify.sh          # lint + fetch dry-run + no-CRD-committed guard
```

Tag releases as `helm-multus-v<semver>` (see `scripts/tag-release.sh`);
the publish workflow runs `ci/fetch.sh` before `helm package`, signs with
cosign, and moves the `stable`/`dev` floating tags.
