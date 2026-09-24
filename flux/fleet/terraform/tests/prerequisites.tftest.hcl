# Barebone-Talos bootstrap wiring: the host-networked Job + Cilium
# prerequisite chart must track the same manifests Flux reconciles
# (flux/infra/components/cilium/controllers/base/cilium.yaml + the
# controllers/<env>/ kustomization VIP patches). Fails `tofu test` on drift.
mock_provider "kubernetes" {}
mock_provider "helm" {}

variables {
  oci_token               = "test-token"
  cluster_name            = "acme-prd-bdo1-talos-apps-01"
  cluster_region          = "home-lab"
  bootstrap_revision      = 1
  cilium_k8s_service_host = "192.168.1.198"
}

# Prd: repository/tag track the OCIRepository doc; values track the
# HelmRelease spec.values with the prd API VIP substituted.
run "cilium_prerequisite_matches_gitops_prd" {
  command = plan

  assert {
    condition     = output.test_cilium_chart_repository == "quay.io/cilium/charts/cilium"
    error_message = "Cilium prerequisite repository must be the OCIRepository url from base/cilium.yaml minus the oci:// prefix (module re-adds it)."
  }

  assert {
    condition     = output.test_cilium_chart_tag == "1.20.2"
    error_message = "Cilium prerequisite tag must equal the OCIRepository ref.tag in base/cilium.yaml."
  }

  assert {
    condition     = output.test_cilium_values["k8sServiceHost"] == "192.168.1.198"
    error_message = "Prd bootstrap must pass the prd Talos API VIP (192.168.1.198), matching the controllers/prd kustomization patch."
  }

  assert {
    condition     = output.test_cilium_values["kubeProxyReplacement"] == true
    error_message = "Cilium prerequisite values must keep kubeProxyReplacement=true (Talos runs with kube-proxy disabled)."
  }

  assert {
    condition     = output.test_cilium_values["k8sServicePort"] == 6443
    error_message = "Cilium prerequisite values must keep k8sServicePort=6443 from the Flux-reconciled HelmRelease."
  }

  # No leftover template placeholder may reach Helm.
  assert {
    condition     = !strcontains(jsonencode(output.test_cilium_values), "__TALOS_API_VIP__")
    error_message = "Cilium prerequisite values must not contain the __TALOS_API_VIP__ placeholder."
  }

  # Steady-state handoff identity: release `cilium` in kube-system (mirrors
  # the Flux tenant targetNamespace/storageNamespace); kube-system
  # pre-exists on Talos so the Job must not create it.
  assert {
    condition     = output.test_cilium_prerequisite.name == "cilium" && output.test_cilium_prerequisite.namespace == "kube-system" && output.test_cilium_prerequisite.create_namespace == false
    error_message = "Cilium prerequisite must be release cilium in kube-system with create_namespace=false (kube-system pre-exists on Talos)."
  }

  # Adoption check: DaemonSet `cilium` in kube-system (chart .Values.name
  # default; `cilium-agent` is only the app.kubernetes.io/name label). The
  # Job skips the chart once helm-controller stamps ownership labels.
  assert {
    condition     = output.test_cilium_prerequisite.adoption.resource == "daemonset" && output.test_cilium_prerequisite.adoption.name == "cilium" && output.test_cilium_prerequisite.adoption.namespace == "kube-system"
    error_message = "flux_adoption_check must target the cilium DaemonSet in kube-system (chart Values.name default is cilium, not cilium-agent)."
  }

  # Barebone Talos: the bootstrap Job must run host-networked (no CNI at
  # Job time) so ghcr.io/quay.io pulls resolve via Talos ResolverConfig.
  assert {
    condition     = output.test_bootstrap_job.host_network == true
    error_message = "Bootstrap Job must set host_network=true on barebone Talos (no pod networking before Cilium)."
  }

  # k8sServiceHost must be the API VIP (.198), never the LB pool VIP (.199).
  assert {
    condition     = output.test_cilium_values["k8sServiceHost"] != "192.168.1.199" && output.test_cilium_values["k8sServiceHost"] != "192.168.1.249"
    error_message = "k8sServiceHost must be the Talos API VIP, never the Cilium LB pool VIP (.199 prd / .249 dev)."
  }

  # Runtime-info seed: ENVIRONMENT/CLUSTER_NAME/CLUSTER_DOMAIN single-sourced
  # from the GitOps runtime-info.yaml, CLUSTER_REGION from the var (matches
  # the GitOps file so pre-Flux and post-Flux agree).
  assert {
    condition     = nonsensitive(output.test_runtime_seed["ENVIRONMENT"]) == "prd" && nonsensitive(output.test_runtime_seed["CLUSTER_NAME"]) == "acme-prd-bdo1-talos-apps-01" && nonsensitive(output.test_runtime_seed["CLUSTER_DOMAIN"]) == "home-ops.yansyah.my.id" && nonsensitive(output.test_runtime_seed["CLUSTER_REGION"]) == "home-lab"
    error_message = "Runtime seed must carry prd ENVIRONMENT/CLUSTER_NAME/CLUSTER_DOMAIN from runtime-info.yaml plus CLUSTER_REGION=home-lab."
  }
}

# Dev: same chart coordinates (single chart tag), dev API VIP substituted.
run "cilium_prerequisite_matches_gitops_dev" {
  command = plan

  variables {
    cluster_name            = "acme-dev-bdo1-talos-apps-01"
    cilium_k8s_service_host = "192.168.1.248"
  }

  assert {
    condition     = output.test_cilium_chart_repository == "quay.io/cilium/charts/cilium"
    error_message = "Cilium prerequisite repository is cluster-independent (single OCIRepository source)."
  }

  assert {
    condition     = output.test_cilium_chart_tag == "1.20.2"
    error_message = "Cilium prerequisite tag is cluster-independent (dev reuses the same chart tag)."
  }

  assert {
    condition     = output.test_cilium_values["k8sServiceHost"] == "192.168.1.248"
    error_message = "Dev bootstrap must pass the dev Talos API VIP (192.168.1.248), matching the controllers/dev kustomization patch."
  }

  assert {
    condition     = output.test_cilium_values["kubeProxyReplacement"] == true
    error_message = "Cilium prerequisite values must keep kubeProxyReplacement=true on dev."
  }

  assert {
    condition     = !strcontains(jsonencode(output.test_cilium_values), "__TALOS_API_VIP__")
    error_message = "Cilium prerequisite values must not contain the __TALOS_API_VIP__ placeholder on dev."
  }

  assert {
    condition     = output.test_cilium_prerequisite.adoption.name == "cilium" && output.test_bootstrap_job.host_network == true
    error_message = "Dev bootstrap keeps the same adoption check (DaemonSet cilium) and host-networked Job."
  }

  assert {
    condition     = output.test_cilium_values["k8sServiceHost"] != "192.168.1.199" && output.test_cilium_values["k8sServiceHost"] != "192.168.1.249"
    error_message = "Dev k8sServiceHost must be the API VIP (.248), never the LB pool VIP."
  }

  assert {
    condition     = nonsensitive(output.test_runtime_seed["ENVIRONMENT"]) == "dev" && nonsensitive(output.test_runtime_seed["CLUSTER_NAME"]) == "acme-dev-bdo1-talos-apps-01" && nonsensitive(output.test_runtime_seed["CLUSTER_DOMAIN"]) == "home-ops-dev.yansyah.my.id" && nonsensitive(output.test_runtime_seed["CLUSTER_REGION"]) == "home-lab"
    error_message = "Runtime seed must carry dev ENVIRONMENT/CLUSTER_NAME/CLUSTER_DOMAIN from runtime-info.yaml plus CLUSTER_REGION=home-lab."
  }
}
