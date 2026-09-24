# Bootstrap the home cluster with the Flux Operator via the
# controlplaneio-fluxcd/flux-operator-bootstrap module.
#
# BAREBONE TALOS: the base machine config ships with NO pod networking, NO
# ClusterIP DNAT, and NO cluster DNS —
#   talos/clusters/_base/patches.yml deletes KubeFlannelCNIConfig (no CNI),
#   sets KubeCoreDNSConfig{enabled:false} (no CoreDNS), and per-cluster
#   patches set KubeProxyConfig{enabled:false} (Cilium replaces kube-proxy).
# At `tofu apply` time the bootstrap Job therefore cannot use normal pod
# networking: `job.host_network = true` runs it on the host stack (upstream
# escape hatch documented for installing a CNI from the Job) and the
# `prerequisites.charts` slot installs Cilium BEFORE the Flux Operator, so
# pod networking + ClusterIP + API reachability exist when Flux takes over.
# What runs pre-Flux vs post-Flux:
#   pre-Flux  (this module): Cilium Helm chart (node networking), Flux
#             Operator chart, FluxInstance. Managed by Terraform/bootstrap Job.
#   post-Flux (Flux tenants): Cilium + CoreDNS HelmReleases via the infra
#             ResourceSet (flux/fleet/tenants/infra.yaml inputs cilium, coredns).
#             Flux adopts the Cilium release (flux_adoption_check below) and
#             the namespace, then owns it; Terraform stops touching it.
# CoreDNS is deliberately NOT a prerequisite: pre-Cilium the Job uses
# host DNS (dnsPolicy Default, Talos ResolverConfig 1.1.1.1/8.8.8.8, so
# ghcr.io/quay.io pulls work); post-Cilium the Job keeps host DNS while
# Flux reconciles CoreDNS as a tenant with no ordering dependency on the Job.
# CoreDNS stays out: the Job never does in-cluster DNS before CoreDNS lands.
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
#   initial bootstrap install. The operator version moves with the
#   flux/apps/components/flux-operator-ui chart tag (see terraform/README.md
#   "Upgrading the operator").
# - prerequisites Cilium chart coordinates (repository + tag) and values <-
#   ../../../infra/components/cilium/controllers/base/cilium.yaml (the same
#   HelmRelease Flux reconciles). main.tf parses that file with yamldecode:
#   repository = OCIRepository url minus the `oci://` prefix (the module
#   re-adds it at install time), tag = OCIRepository ref.tag, values =
#   HelmRelease spec.values re-encoded with the per-cluster
#   var.cilium_k8s_service_host substituted for the __TALOS_API_VIP__
#   placeholder. Asserted by tests/prerequisites.tftest.hcl; `tofu test`
#   fails on drift.
# After the bootstrap Job completes, Flux owns the operator HelmRelease and
# the FluxInstance; Terraform only re-runs the Job on input change or when
# bootstrap_revision is bumped.
#
# FIRST BOOTSTRAP ORDER: the prd FluxInstance syncs ref `stable`, which
# does not exist until the first flux-fleet-vX.Y.Z release is tagged.
# Bootstrap the DEV cluster first (syncs ref `dev`, published from every
# main commit), validate end to end, then tag the release and bootstrap
# prd pinned to it. See README.md "First bootstrap order". The Job itself
# is ref-agnostic (consumes local files, never pulls the OCI tag).
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

  # Cilium prerequisite, parsed from the SAME manifests Flux reconciles
  # (flux/infra/components/cilium/controllers/base/cilium.yaml). Docs are
  # selected by kind (not position) so comment blocks and doc order changes
  # cannot shift the parse; `one()` fails loudly if a kind is missing or
  # duplicated.
  cilium_manifests = [
    for d in split("\n---\n", replace(file("${path.root}/../../infra/components/cilium/controllers/base/cilium.yaml"), "\n---", "\n---\n")) :
    yamldecode(d) if can(yamldecode(d)) && yamldecode(d) != null
  ]
  cilium_chart_repo_doc = one([for d in local.cilium_manifests : d if try(d.kind, "") == "OCIRepository"])
  cilium_release_doc    = one([for d in local.cilium_manifests : d if try(d.kind, "") == "HelmRelease"])

  cilium_chart_repository = trimprefix(local.cilium_chart_repo_doc.spec.url, "oci://")
  cilium_chart_tag        = local.cilium_chart_repo_doc.spec.ref.tag

  # HelmRelease spec.values re-encoded for the prerequisite chart, with the
  # per-cluster Talos API VIP substituted for the __TALOS_API_VIP__
  # placeholder (same value the controllers/<env>/ kustomization patches in
  # per environment: prd .198, dev .248). Only the scalar
  # k8sServiceHost key carries the placeholder, so merge it explicitly —
  # a generic value-walk would break HCL type unification on nested maps.
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
    # Steady-state handoff: the Flux tenant HelmRelease (releaseName
    # cilium, targetNamespace/storageNamespace kube-system) stamps
    # helm.toolkit.fluxcd.io/name on the `cilium` DaemonSet once it
    # reconciles (chart .Values.name default is `cilium`, NOT
    # `cilium-agent` — that is only the app.kubernetes.io/name label).
    # From then on the Job skips this chart so it never fights
    # helm-controller. kube-system already exists on Talos, so the Job
    # must not create it (create_namespace = false).
    flux_adoption_check = {
      resource  = "daemonset"
      api_group = "apps"
      name      = "cilium"
      namespace = "kube-system"
    }
  }

  # Bootstrap Job network posture, single-sourced here so tests can pin it:
  # host_network=true is required on barebone Talos (see module call below).
  bootstrap_job = {
    host_network = true
  }

  # Runtime-info seed for the bootstrap Job's flux-runtime-info ConfigMap.
  # Single-sourced from the SAME runtime-info.yaml Flux reconciles after
  # bootstrap (clusters/<cluster_name>/flux-system/runtime-info.yaml), plus
  # CLUSTER_REGION from var.cluster_region (var wins on conflict). The Job
  # needs this ConfigMap pre-Flux for `flux envsubst --strict`; after Flux
  # takes over, the GitOps file (which also carries CLUSTER_REGION) is the
  # owner and the copyFrom + postBuild.substituteFrom chain fans the keys
  # out to every tenant namespace.
  flux_runtime_seed = merge(
    yamldecode(file("${path.root}/../clusters/${var.cluster_name}/flux-system/runtime-info.yaml")).data,
    { CLUSTER_REGION = var.cluster_region }
  )
}

module "flux_operator_bootstrap" {
  source  = "controlplaneio-fluxcd/flux-operator-bootstrap/kubernetes"
  version = local.flux_operator_ref.bootstrap_module_version

  revision = var.bootstrap_revision

  # host_network: required on barebone Talos — without a CNI there is no pod
  # networking, so the Job must run on the host stack (upstream: "required
  # when the job must install a CNI plugin"). Implies dnsPolicy Default, so
  # ghcr.io/quay.io pulls resolve via the node's resolvers (see header note
  # on why CoreDNS stays a Flux tenant instead of a prerequisite).
  job = local.bootstrap_job

  gitops_resources = {
    instance_yaml = file("${path.root}/../clusters/${var.cluster_name}/flux-system/flux-instance.yaml")
    operator_chart = {
      repository  = local.flux_operator_ref.operator_chart_repository
      version     = local.flux_operator_ref.operator_chart_version
      values_yaml = file("${path.root}/../clusters/${var.cluster_name}/flux-system/flux-operator-values.yaml")
    }
    prerequisites = {
      # prerequisites.charts[0] MUST stay cilium: the bootstrap Job installs
      # node networking before the Flux Operator on barebone Talos.
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
