output "org_id" {
  description = "home-ops org ID"
  value       = zitadel_org.home_ops.id
}

output "project_id" {
  description = "home-ops project ID"
  value       = zitadel_project.home_ops.id
}

output "client_ids" {
  description = "OIDC client_id per client (store secrets from state into Proton Pass)"
  value       = { for k, v in zitadel_application_oidc.clients : k => v.client_id }
  sensitive   = true
}
