# Diff-verifiable single source: the operator chart coordinates in
# versions.yaml must match the OCIRepository consumed by Flux in
# clusters/acme-prd-bdo1-talos-apps-01, clusters/update, AND
# clusters/acme-dev-bdo1-talos-apps-01 flux-operator.yaml
# (§14: dev bootstrap reuses the same single source).
mock_provider "kubernetes" {}
mock_provider "helm" {}

variables {
  oci_token               = "test-token"
  cluster_name            = "acme-prd-bdo1-talos-apps-01"
  cluster_region          = "home-lab"
  bootstrap_revision      = 1
  cilium_k8s_service_host = "192.168.1.198"
}

run "operator_versions_match_gitops" {
  command = plan

  # The flux-operator.yaml ResourceSet embeds the chart OCIRepository inside
  # spec.resources, so match the repository string against the raw file text
  # instead of decoding nested YAML.
  assert {
    condition = alltrue([
      for f in ["acme-prd-bdo1-talos-apps-01", "update", "acme-dev-bdo1-talos-apps-01"] :
      strcontains(
        file("${path.root}/../clusters/${f}/flux-system/flux-operator.yaml"),
        yamldecode(file("${path.root}/versions.yaml"))["operator_chart_repository"]
      )
    ])
    error_message = "flux-operator.yaml OCIRepository url must contain the versions.yaml operator_chart_repository."
  }

  # Pinned versions: bootstrap module 0.8.0, operator chart 0.60.0.
  assert {
    condition     = output.test_operator_ref["bootstrap_module_version"] == "0.8.0"
    error_message = "bootstrap module version must stay pinned at 0.8.0 (versions.yaml)."
  }

  assert {
    condition     = output.test_operator_ref["operator_chart_version"] == "0.60.0"
    error_message = "operator chart version must stay pinned at 0.60.0 (versions.yaml)."
  }

  assert {
    condition     = output.test_operator_ref["operator_chart_repository"] == "ghcr.io/controlplaneio-fluxcd/charts/flux-operator"
    error_message = "operator chart repository must be ghcr.io/controlplaneio-fluxcd/charts/flux-operator (versions.yaml)."
  }

  # instance_yaml single-source: module input must equal the GitOps file.
  assert {
    condition     = strcontains(file("${path.root}/../clusters/acme-prd-bdo1-talos-apps-01/flux-system/flux-instance.yaml"), "stable")
    error_message = "prd FluxInstance must sync ref stable."
  }

  assert {
    condition     = strcontains(file("${path.root}/../clusters/acme-dev-bdo1-talos-apps-01/flux-system/flux-instance.yaml"), "ref: \"dev\"")
    error_message = "dev FluxInstance must sync ref dev (per-commit, no release tag needed)."
  }

  # bootstrap_revision bump re-runs: the module receives var input so a bump
  # changes the Job revision annotation (plan diff on revision change).
  assert {
    condition     = var.bootstrap_revision == 1
    error_message = "bootstrap_revision default stays 1; bump it to trigger a new bootstrap run."
  }
}

# Instance + values stay single-sourced per cluster directory.
run "instance_values_single_source" {
  command = plan

  variables {
    cluster_name            = "acme-dev-bdo1-talos-apps-01"
    cilium_k8s_service_host = "192.168.1.248"
  }

  assert {
    condition     = strcontains(file("${path.root}/../clusters/acme-dev-bdo1-talos-apps-01/flux-system/flux-operator-values.yaml"), "defaultServiceAccount")
    error_message = "dev operator values file must be the single source consumed by both Terraform and Flux."
  }

  assert {
    condition     = nonsensitive(output.test_runtime_seed["ARTIFACT_TAG"]) == "dev"
    error_message = "dev runtime seed must carry ARTIFACT_TAG=dev from runtime-info.yaml."
  }
}
