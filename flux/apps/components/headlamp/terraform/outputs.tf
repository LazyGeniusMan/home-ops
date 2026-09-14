output "client_id" {
  description = "Generated OIDC client_id for the headlamp client (lands in the headlamp-sso-outputs Secret; consumed by the headlamp-oidc ExternalSecret via the headlamp-k8s SecretStore)"
  value       = module.sso.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the headlamp client (lands in the headlamp-sso-outputs Secret; consumed by the headlamp-oidc ExternalSecret via the headlamp-k8s SecretStore)"
  value       = module.sso.client_secret
  sensitive   = true
}

output "project_id" {
  description = "ID of the headlamp Zitadel project owned by this slice"
  value       = module.sso.project_id
}
