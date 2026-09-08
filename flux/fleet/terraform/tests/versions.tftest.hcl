# Diff-verifiable single source: the operator chart coordinates in
# versions.yaml must match the OCIRepository consumed by Flux in
# clusters/home/flux-system/flux-operator.yaml and clusters/update/flux-system/flux-operator.yaml.
mock_provider "kubernetes" {}
mock_provider "helm" {}

variables {
  oci_token          = "test-token"
  cluster_name       = "home"
  cluster_region     = "home-lab"
  bootstrap_revision = 1
}

run "operator_versions_match_gitops" {
  command = plan

  # The flux-operator.yaml ResourceSet embeds the chart OCIRepository inside
  # spec.resources, so match the repository string against the raw file text
  # instead of decoding nested YAML.
  assert {
    condition = alltrue([
      for f in ["home", "update"] :
      strcontains(
        file("${path.root}/../clusters/${f}/flux-system/flux-operator.yaml"),
        yamldecode(file("${path.root}/versions.yaml"))["operator_chart_repository"]
      )
    ])
    error_message = "flux-operator.yaml OCIRepository url must contain the versions.yaml operator_chart_repository."
  }
}
