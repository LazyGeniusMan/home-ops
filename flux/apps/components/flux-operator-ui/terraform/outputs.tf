output "client_id" {
  description = "Generated OIDC client_id for the flux-operator-ui client (lands in the flux-operator-ui-sso-outputs Secret; consumed by the oauth2-proxy-credentials ExternalSecret via the flux-operator-ui-k8s SecretStore)"
  value       = module.sso.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the flux-operator-ui client (lands in the flux-operator-ui-sso-outputs Secret; consumed by the oauth2-proxy-credentials ExternalSecret via the flux-operator-ui-k8s SecretStore)"
  value       = module.sso.client_secret
  sensitive   = true
}
