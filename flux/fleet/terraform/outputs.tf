# Test-only hooks for tests/*.tftest.hcl (no runtime meaning).

output "test_cilium_chart_repository" {
  description = "Cilium prerequisite chart repository passed to the bootstrap module (test hook)."
  value       = local.cilium_chart_repository
}

output "test_cilium_chart_tag" {
  description = "Cilium prerequisite chart tag passed to the bootstrap module (test hook)."
  value       = local.cilium_chart_tag
}

output "test_cilium_values" {
  description = "Cilium prerequisite chart values passed to the bootstrap module (test hook)."
  value       = local.cilium_values
}

output "test_cilium_prerequisite" {
  description = "Full Cilium prerequisite chart object passed to the bootstrap module (test hook)."
  value = {
    name             = local.cilium_prerequisite.name
    namespace        = local.cilium_prerequisite.namespace
    create_namespace = local.cilium_prerequisite.create_namespace
    adoption         = local.cilium_prerequisite.flux_adoption_check
  }
}

output "test_bootstrap_job" {
  description = "Bootstrap Job settings passed to the module (test hook)."
  value       = local.bootstrap_job
}

output "test_runtime_seed" {
  description = "Runtime-info seed passed to the module (test hook)."
  value       = local.flux_runtime_seed
  sensitive   = true
}

output "test_operator_ref" {
  description = "Operator chart/module versions from versions.yaml (test hook)."
  value       = local.flux_operator_ref
}
