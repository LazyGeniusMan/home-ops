# Test-only hooks: expose the Cilium prerequisite wiring so
# tests/prerequisites.tftest.hcl can assert the single-source mapping
# (module inputs are not otherwise readable from test assertions).
# These carry no runtime meaning — the module owns the real install.

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
