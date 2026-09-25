# Bootstrap the cluster via the controlplaneio-fluxcd/flux-operator-bootstrap
# module. Talos ships barebone (no CNI/DNS): the Job runs host-networked,
# installs Cilium from `prerequisites` before the operator; CoreDNS follows
# via Flux. Single source: instance/values/operator coords read the same
# files Flux reconciles (see README.md); Cilium coords parsed from
# cilium/controllers/base/cilium.yaml with __TALOS_API_VIP__ filled per
# cluster. Bootstrap dev first — prd syncs `stable`, which exists only after
# the first flux-fleet-vX.Y.Z release (see README.md "First bootstrap order").
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

  # Cilium prerequisite, parsed from the same manifests Flux reconciles.
  # Docs selected by kind (not position); `one()` fails if a kind is
  # missing or duplicated.
  cilium_manifests = [
    for d in split("\n---\n", replace(file("${path.root}/../../infra/components/cilium/controllers/base/cilium.yaml"), "\n---", "\n---\n")) :
    yamldecode(d) if can(yamldecode(d)) && yamldecode(d) != null
  ]
  cilium_chart_repo_doc = one([for d in local.cilium_manifests : d if try(d.kind, "") == "OCIRepository"])
  cilium_release_doc    = one([for d in local.cilium_manifests : d if try(d.kind, "") == "HelmRelease"])

  cilium_chart_repository = trimprefix(local.cilium_chart_repo_doc.spec.url, "oci://")
  cilium_chart_tag        = local.cilium_chart_repo_doc.spec.ref.tag

  # HelmRelease spec.values re-encoded with the per-cluster Talos API VIP
  # (prd .198, dev .248) substituted for __TALOS_API_VIP__ (k8sServiceHost
  # only; explicit merge avoids HCL type-unification breaks).
  cilium_values = merge(
    { for k, v in local.cilium_release_doc.spec.values : k => v if k != "k8sServiceHost" },
    { k8sServiceHost = var.cilium_k8s_service_host }
  )

  cilium_prerequisite = {
    name             = "cilium"
    repository       = local.cilium_chart_repository
    version          = local.cilium_chart_tag
    namespace        = "kube-system"
    create_namespace = false
    values_yaml      = yamlencode(local.cilium_values)
    # Handoff: Flux stamps helm.toolkit.fluxcd.io/name on the `cilium`
    # DaemonSet once reconciled (chart Values.name default `cilium`, not
    # `cilium-agent`); the Job then skips the chart. kube-system pre-exists
    # on Talos, so create_namespace = false.
    flux_adoption_check = {
      resource  = "daemonset"
      api_group = "apps"
      name      = "cilium"
      namespace = "kube-system"
    }
  }

  # Bootstrap Job network posture, single-sourced here so tests can pin it.
  bootstrap_job = {
    host_network = true
  }

  # Runtime-info seed: same runtime-info.yaml Flux reconciles after
  # bootstrap, plus CLUSTER_REGION from var.cluster_region (var wins).
  flux_runtime_seed = merge(
    yamldecode(file("${path.root}/../clusters/${var.cluster_name}/flux-system/runtime-info.yaml")).data,
    { CLUSTER_REGION = var.cluster_region }
  )
}

module "flux_operator_bootstrap" {
  source  = "controlplaneio-fluxcd/flux-operator-bootstrap/kubernetes"
  version = local.flux_operator_ref.bootstrap_module_version

  revision = var.bootstrap_revision

  # host_network: required on barebone Talos (no CNI at Job time);
  # implies dnsPolicy Default.
  job = local.bootstrap_job

  gitops_resources = {
    instance_yaml = file("${path.root}/../clusters/${var.cluster_name}/flux-system/flux-instance.yaml")
    operator_chart = {
      repository  = local.flux_operator_ref.operator_chart_repository
      version     = local.flux_operator_ref.operator_chart_version
      values_yaml = file("${path.root}/../clusters/${var.cluster_name}/flux-system/flux-operator-values.yaml")
    }
    prerequisites = {
      # prerequisites.charts[0] stays cilium (node networking before the operator).
      charts = [local.cilium_prerequisite]
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
      data = local.flux_runtime_seed
    }
  }
}
