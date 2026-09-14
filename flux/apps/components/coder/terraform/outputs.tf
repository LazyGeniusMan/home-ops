output "client_id" {
  description = "Generated OIDC client_id for the coder client (lands in the coder-sso-outputs Secret; consumed by the coder-oidc ExternalSecret via the coder-k8s SecretStore)"
  value       = module.sso.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the coder client (lands in the coder-sso-outputs Secret; consumed by the coder-oidc ExternalSecret via the coder-k8s SecretStore)"
  value       = module.sso.client_secret
  sensitive   = true
}

output "project_id" {
  description = "ID of the coder Zitadel project owned by this slice"
  value       = module.sso.project_id
}
