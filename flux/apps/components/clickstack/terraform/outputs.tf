output "client_id" {
  description = "Generated OIDC client_id for the clickstack client (lands in the clickstack-sso-outputs Secret; consumed by the oauth2-proxy ExternalSecret via the clickstack-k8s SecretStore)"
  value       = zitadel_application_oidc.clickstack.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the clickstack client (lands in the clickstack-sso-outputs Secret; consumed by the oauth2-proxy ExternalSecret via the clickstack-k8s SecretStore)"
  value       = zitadel_application_oidc.clickstack.client_secret
  sensitive   = true
}
