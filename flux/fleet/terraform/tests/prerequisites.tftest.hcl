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
    condition     = output.test_cilium_chart_tag == "1.20.1"
    error_message = "Cilium prerequisite tag must equal the OCIRepository ref.tag in base/cilium.yaml."
  }

  assert {
    condition     = output.test_cilium_values["k8sServiceHost"] == "192.168.1.198"
    error_message = "Prd bootstrap must pass the prd Talos API VIP (192.168.1.198), matching controllers/prd + controllers/stg kustomization patches."
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
    condition     = output.test_cilium_chart_tag == "1.20.1"
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
}
