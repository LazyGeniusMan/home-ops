output "client_id" {
  description = "Generated OIDC client_id for the headlamp client (lands in the headlamp-sso-outputs Secret; consumed by the headlamp-oidc ExternalSecret via the headlamp-k8s SecretStore)"
  value       = zitadel_application_oidc.headlamp.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the headlamp client (lands in the headlamp-sso-outputs Secret; consumed by the headlamp-oidc ExternalSecret via the headlamp-k8s SecretStore)"
  value       = zitadel_application_oidc.headlamp.client_secret
  sensitive   = true
}
