# Fleet sync-breakers: tenant source allowlist + cosign verify identities +
# image-automation cadence. Guards the findings that break sync outright: a
# chart registry missing from the policies.yaml allowlist denies the tenant
# chart OCIRepositories (ValidatingAdmissionPolicy on tenant namespaces), a
# release-only cosign subject denies every dev sync (dev tags are signed by
# the push workflow on refs/heads/main, not the release workflow), and a
# changed automation interval/branch breaks the update PR flow.
mock_provider "kubernetes" {}
mock_provider "helm" {}

variables {
  oci_token               = "test-token"
  cluster_name            = "acme-prd-bdo1-talos-apps-01"
  cluster_region          = "home-lab"
  bootstrap_revision      = 1
  cilium_k8s_service_host = "192.168.1.198"
}

# Allowlist must prefix-match every chart OCIRepository spec.url used by
# components (scanned from all spec.url in flux/{infra,apps}/components;
# ferretdb/matrix-construct ship image refs only, no OCIRepository spec.url).
# Kept in sync with the exhaustive source list in tenants/policies.yaml.
run "tenant_source_allowlist_covers_charts" {
  command = plan

  assert {
    condition = alltrue([
      for prefix in [
        "oci://ghcr.io/lazygeniusman/",
        "oci://ghcr.io/coder/",
        "oci://ghcr.io/oauth2-proxy/",
        "oci://ghcr.io/coredns/",
        "oci://ghcr.io/dragonflydb/",
        "oci://ghcr.io/external-secrets/",
        "oci://ghcr.io/cloudnative-pg/",
        "oci://ghcr.io/rancher/",
        "oci://ghcr.io/zitadel/",
        "oci://ghcr.io/controlplaneio-fluxcd/",
        "oci://ghcr.io/flux-iac/",
        "oci://quay.io/jetstack/charts/",
        "oci://quay.io/cilium/charts/",
        "oci://registry-1.docker.io/altinity/",
        "oci://chartproxy.container-registry.com/",
      ] : strcontains(file("${path.root}/../tenants/policies.yaml"), prefix)
    ])
    error_message = "tenants/policies.yaml allowlist must prefix-match every chart registry used by components (coder, oauth2-proxy, coredns, dragonfly, external-secrets, cnpg, local-path-provisioner, zitadel, fluxcd/flux-iac, cert-manager, cilium, clickhouse-operator, chartproxy); a missing prefix denies the tenant sync."
  }
}

# Tenant OCIRepositories must verify BOTH the push identity (dev tag, signed
# per-commit on refs/heads/main) and the release identity (stable tag, signed
# on version tags); a release-only subject denies every dev sync.
run "tenant_cosign_identities_cover_dev_and_stable" {
  command = plan

  assert {
    condition     = strcontains(file("${path.root}/../tenants/apps.yaml"), "flux-apps-push\\.yaml@refs/heads/main") && strcontains(file("${path.root}/../tenants/apps.yaml"), "inputs.artifactSubjectWorkflow")
    error_message = "tenants/apps.yaml OCIRepository verify must list both the flux-apps-push (dev) and the release-workflow (stable) identities."
  }

  assert {
    condition     = strcontains(file("${path.root}/../tenants/infra.yaml"), "flux-infra-push\\.yaml@refs/heads/main") && strcontains(file("${path.root}/../tenants/infra.yaml"), "inputs.artifactSubjectWorkflow")
    error_message = "tenants/infra.yaml OCIRepository verify must list both the flux-infra-push (dev) and the release-workflow (stable) identities."
  }
}

# Cluster FluxInstances verify the identity matching their tag: dev + update
# sync tag dev (signed by flux-fleet-push), prd syncs stable (signed by
# flux-fleet-release).
run "cluster_verify_identities_match_tag" {
  command = plan

  assert {
    condition     = strcontains(file("${path.root}/../clusters/acme-dev-bdo1-talos-apps-01/flux-system/flux-instance.yaml"), "workflows/flux-fleet-push\\.yaml@refs/heads/main") && !strcontains(file("${path.root}/../clusters/acme-dev-bdo1-talos-apps-01/flux-system/flux-instance.yaml"), "workflows/flux-fleet-release")
    error_message = "dev FluxInstance syncs tag dev so it must verify flux-fleet-push@refs/heads/main, never flux-fleet-release."
  }

  assert {
    condition     = strcontains(file("${path.root}/../clusters/acme-prd-bdo1-talos-apps-01/flux-system/flux-instance.yaml"), "flux-fleet-release")
    error_message = "prd FluxInstance syncs tag stable so it must verify flux-fleet-release."
  }

  assert {
    condition     = strcontains(file("${path.root}/../clusters/update/flux-system/flux-instance.yaml"), "flux-fleet-push\\.yaml@refs/heads/main")
    error_message = "update FluxInstance syncs tag dev so it must verify flux-fleet-push@refs/heads/main."
  }
}

# ImageUpdateAutomation stays intact: 30m interval, Setters strategy pushing
# image-updates-* branches for both areas (automation proposes, human merges).
run "image_update_automation_intact" {
  command = plan

  assert {
    condition     = strcontains(file("${path.root}/../clusters/update/automation.yaml"), "interval: 30m") && strcontains(file("${path.root}/../clusters/update/automation.yaml"), "strategy: Setters")
    error_message = "automation.yaml ImageUpdateAutomation must keep interval 30m with the Setters strategy."
  }

  assert {
    condition     = strcontains(file("${path.root}/../clusters/update/automation.yaml"), "image-updates-apps") && strcontains(file("${path.root}/../clusters/update/automation.yaml"), "image-updates-infra")
    error_message = "automation.yaml must keep pushing image-updates-apps + image-updates-infra branches."
  }
}
