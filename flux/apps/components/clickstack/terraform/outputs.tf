output "client_id" {
  description = "Generated OIDC client_id for the clickstack client (lands in the clickstack-sso-outputs Secret; consumed by the oauth2-proxy ExternalSecret via the clickstack-k8s SecretStore)"
  value       = module.sso.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the clickstack client (lands in the clickstack-sso-outputs Secret; consumed by the oauth2-proxy ExternalSecret via the clickstack-k8s SecretStore)"
  value       = module.sso.client_secret
  sensitive   = true
}
