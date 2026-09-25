# Flux bootstrap (Terraform)

Bootstraps the Flux Operator with OpenTofu/Terraform; Flux owns steady state
after. No workloads — desired state is `../clusters/<cluster_name>`.
Upstream refs: `/tmp/home-ops-docs/flux-operator-bootstrap-terraform-docs`, `/tmp/home-ops-docs/helm-docs`.

## Prerequisites

Talos ships barebone (no CNI, no CoreDNS, no kube-proxy), so the bootstrap
Job runs host-networked (`job.host_network = true`, `dnsPolicy: Default`)
and installs the Cilium chart from `prerequisites.charts` before the Flux
Operator; Cilium + CoreDNS then reconcile as infra tenants
(`../tenants/infra.yaml`) with Flux adopting the Cilium release via
`flux_adoption_check`. CoreDNS is not a prerequisite — the Job uses host
DNS throughout. Only per-cluster difference: `var.cilium_k8s_service_host`
(Talos API VIP `k8sServiceHost`: `.198` prd, `.248` dev; not the LB pool VIP `.199`/`.249`).

## Single source of truth

`main.tf` reads the same files Flux reconciles (no duplicated versions/values):

- `gitops_resources.instance_yaml` ← `../clusters/<cluster_name>/flux-system/flux-instance.yaml`
- `operator_chart.values_yaml` ← `../clusters/<cluster_name>/flux-system/flux-operator-values.yaml`
- `operator_chart.repository`/`version` ← `versions.yaml` (matches the `OCIRepository` url in `flux-operator.yaml`; GitOps floats semver `*`, `versions.yaml` pins the bootstrap install)
- `prerequisites.charts[0]` ← `../../infra/components/cilium/controllers/base/cilium.yaml` (same OCIRepository + HelmRelease; `k8sServiceHost` from `var.cilium_k8s_service_host`)

`tests/*.tftest.hcl` assert both mappings; `tofu test` fails on drift.

## Runtime info flow

`clusters/<name>/flux-system/runtime-info.yaml` (`ARTIFACT_TAG`,
`ENVIRONMENT`, `CLUSTER_NAME`, `CLUSTER_DOMAIN`, `CLUSTER_REGION`) is the
single source: Terraform seeds the Job's ConfigMap pre-Flux
(`var.cluster_region` must match the file); post-Flux the GitOps file owns
it and ResourceSets fan it out via `copyFrom` + `postBuild.substituteFrom`
(`${ENVIRONMENT}` selects `controllers/<env>/`). The `update` cluster is
automation only (no `CLUSTER_DOMAIN` — nothing under `clusters/update`
substitutes it; `CLUSTER_REGION` is real so the seed stays single-sourced).

`managed_resources.secrets_yaml` ships `flux-system/ghcr-auth` always, plus
`flux-system/github-auth` (username `home-ops-bot`) only when
`var.github_token` is set — pass it when bootstrapping the `update` cluster
so `ImageUpdateAutomation` can push `image-updates-*` branches
(`automation.yaml` `copyFrom`s it into the `apps`/`infra` namespaces).

## First bootstrap order

The prd `FluxInstance` syncs `ref: stable`, which exists only after the
first `flux-fleet-vX.Y.Z` release — bootstrap dev first:

1. **Bootstrap dev** (`cluster_name=acme-dev-bdo1-talos-apps-01`,
   `cilium_k8s_service_host=192.168.1.248`; syncs `ref: dev` from every
   `main` commit). Validate: Cilium → Operator → infra tenants.
2. **Publish stable** (`flux-fleet-vX.Y.Z`; pushes cosigned `stable` + `<version>`).
3. **Bootstrap prd** (`cluster_name=acme-prd-bdo1-talos-apps-01`,
   `cilium_k8s_service_host=192.168.1.198`).

The Job is ref-agnostic (local files only). Later bootstraps skip to step 3.

## Upgrading the operator

Three pins move together: `operator_chart_version` in `versions.yaml`
(0.60.0, bootstrap installs only — GitOps floats semver `*`), the chart tag
in `flux/apps/components/flux-operator-ui` (same line `>=0.60.0`,
`$imagepolicy`), and `FluxInstance` `spec.distribution.version` (`2.x`,
keep the major). `tests/versions.tftest.hcl` asserts the mapping (module
0.8.0, chart 0.60.0, prd `stable` / dev `dev`); `tofu test` fails on drift.

## Usage (manual, no live apply in CI)

```shell
cd flux/fleet/terraform
tofu init -backend=false && tofu validate && tofu test
tofu plan -var oci_token="${GITHUB_TOKEN}" \
  -var cluster_name="acme-prd-bdo1-talos-apps-01" \
  -var cluster_region="home-lab" \
  -var cilium_k8s_service_host="192.168.1.198"
```

Use `.248` for dev. For the `update` cluster bootstrap add
`-var cluster_name="update" -var github_token="<contents-read-write-token>"`
(`cilium_k8s_service_host` is still required by validation — pass the dev
`.248` value; the update host is not Talos-managed, so review the Cilium
prerequisite effect on that host before applying).
`tofu plan` needs no live cluster (dummy kubeconfig OK).
Bump `var.bootstrap_revision` for a new bootstrap run.
