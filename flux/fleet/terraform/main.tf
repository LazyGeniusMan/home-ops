# Bootstrap the home cluster with the Flux Operator via the
# controlplaneio-fluxcd/flux-operator-bootstrap module.
#
# SINGLE SOURCE (versions + values): this module reads the SAME files that
# Flux reconciles after bootstrap — no version or values may be duplicated
# here. Diff-verifiable mapping:
# - gitops_resources.instance_yaml  <- ../clusters/<cluster_name>/flux-system/flux-instance.yaml
# - gitops_resources.operator_chart.values_yaml <- ../clusters/<cluster_name>/flux-system/flux-operator-values.yaml
# - operator chart repository/version <- versions.yaml (single source read by
#   main.tf); the repository MUST equal the OCIRepository url in
#   ../clusters/<cluster_name>/flux-system/flux-operator.yaml (asserted by
#   tests/versions.tftest.hcl). The GitOps OCIRepository itself tracks
#   semver '*' per D2; versions.yaml records the chart version used for the
#   initial bootstrap install.
# After the bootstrap Job completes, Flux owns the operator HelmRelease and
# the FluxInstance; Terraform only re-runs the Job on input change or when
# bootstrap_revision is bumped.
terraform {
  required_version = ">= 1.11"

  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 3.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.0"
    }
  }
}

locals {
  ghcr_auth_dockerconfigjson = jsonencode({
    auths = {
      "ghcr.io" = {
        username = "flux"
        password = var.oci_token
        auth     = base64encode("flux:${var.oci_token}")
      }
    }
  })

  flux_operator_ref = yamldecode(file("${path.root}/versions.yaml"))
}

module "flux_operator_bootstrap" {
  source  = "controlplaneio-fluxcd/flux-operator-bootstrap/kubernetes"
  version = local.flux_operator_ref.bootstrap_module_version

  revision = var.bootstrap_revision

  gitops_resources = {
    instance_yaml = file("${path.root}/../clusters/${var.cluster_name}/flux-system/flux-instance.yaml")
    operator_chart = {
      repository  = local.flux_operator_ref.operator_chart_repository
      version     = local.flux_operator_ref.operator_chart_version
      values_yaml = file("${path.root}/../clusters/${var.cluster_name}/flux-system/flux-operator-values.yaml")
    }
  }

  managed_resources = {
    secrets_yaml = <<-YAML
      apiVersion: v1
      kind: Secret
      metadata:
        name: ghcr-auth
      type: kubernetes.io/dockerconfigjson
      stringData:
        .dockerconfigjson: '${replace(local.ghcr_auth_dockerconfigjson, "'", "''")}'
    YAML
    runtime_info = {
      data = {
        CLUSTER_REGION = var.cluster_region
      }
    }
  }
}
