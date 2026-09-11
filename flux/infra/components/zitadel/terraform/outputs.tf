output "org_id" {
  description = "home-ops org ID"
  value       = zitadel_org.home_ops.id
}

output "project_id" {
  description = "home-ops project ID"
  value       = zitadel_project.home_ops.id
}

output "admin_user_id" {
  description = "admin human user ID (non-sensitive; per-app slices take this via varsFrom, never ESO)"
  value       = zitadel_human_user.admin.id
}

