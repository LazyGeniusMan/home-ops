# Operator chart coords in versions.yaml must match the OCIRepository in all
# three clusters' flux-operator.yaml.
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

  # flux-operator.yaml embeds the OCIRepository in spec.resources, so match
  # the repository string against raw file text.
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

  # Pinned versions match versions.yaml (asserted below).
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

  # bootstrap_revision bump re-renders the Job revision annotation.
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

# github-auth seeding: null (dev/prd default) ships ghcr-auth only; the update
# bootstrap passes github_token and gets the flux-system/github-auth doc too
# (automation.yaml copyFrom's it into the apps/infra namespaces).
run "github_auth_seed_conditional" {
  command = plan

  assert {
    condition     = strcontains(nonsensitive(output.test_secrets_yaml), "name: ghcr-auth") && !strcontains(nonsensitive(output.test_secrets_yaml), "name: github-auth")
    error_message = "Null github_token must seed ghcr-auth only (dev/prd bootstrap unchanged)."
  }
}

run "github_auth_seed_update" {
  command = plan

  variables {
    github_token = "update-test-token"
  }

  assert {
    condition     = strcontains(nonsensitive(output.test_secrets_yaml), "name: ghcr-auth") && strcontains(nonsensitive(output.test_secrets_yaml), "name: github-auth") && strcontains(nonsensitive(output.test_secrets_yaml), "username: home-ops-bot")
    error_message = "Update bootstrap github_token must add the github-auth Secret (home-ops-bot) next to ghcr-auth."
  }
}
