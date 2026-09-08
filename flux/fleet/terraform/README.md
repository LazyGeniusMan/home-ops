# Flux bootstrap (Terraform)

Bootstraps the Flux Operator on a cluster with OpenTofu/Terraform, then hands
steady-state reconciliation to Flux. No workload content lives here — the
desired state is `../clusters/<cluster_name>`.

## Single source of truth

`main.tf` reuses the same files Flux reconciles (no duplicated versions or
values):

| Terraform input | Source file (also reconciled by Flux) |
| --- | --- |
| `gitops_resources.instance_yaml` | `../clusters/<cluster_name>/flux-system/flux-instance.yaml` |
| `operator_chart.values_yaml` | `../clusters/<cluster_name>/flux-system/flux-operator-values.yaml` |
| `operator_chart.repository` / `version` | `versions.yaml` (repository must match the `OCIRepository` url in `../clusters/<cluster_name>/flux-system/flux-operator.yaml`; the GitOps ref itself tracks semver `*` per D2, `versions.yaml` records the bootstrap install version) |

`tests/versions.tftest.hcl` asserts the mapping; `tofu test` fails on drift.

## Usage (manual, no live apply in CI)

```shell
cd flux/fleet/terraform
tofu init -backend=false
tofu validate
tofu test
tofu plan \
  -var oci_token="${GITHUB_TOKEN}" \
  -var cluster_name="acme-prd-bdo1-talos-apps-01" \
  -var cluster_region="home-lab"
```

Bump `var.bootstrap_revision` to trigger a new bootstrap run.
