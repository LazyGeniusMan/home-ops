output "client_id" {
  description = "Generated OIDC client_id for the coder client (lands in the coder-sso-outputs Secret; consumed by the coder-oidc ExternalSecret via the coder-k8s SecretStore)"
  value       = zitadel_application_oidc.coder.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the coder client (lands in the coder-sso-outputs Secret; consumed by the coder-oidc ExternalSecret via the coder-k8s SecretStore)"
  value       = zitadel_application_oidc.coder.client_secret
  sensitive   = true
}
