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
}
