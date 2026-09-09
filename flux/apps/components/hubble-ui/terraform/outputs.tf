output "client_id" {
  description = "Generated OIDC client_id for the hubble client (consumed natively from the hubble-ui-sso-outputs Secret via the k8s-provider ExternalSecret)"
  value       = zitadel_application_oidc.hubble.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the hubble client (consumed natively from the hubble-ui-sso-outputs Secret via the k8s-provider ExternalSecret)"
  value       = zitadel_application_oidc.hubble.client_secret
  sensitive   = true
}

output "cookie_secret" {
  description = "Generated oauth2-proxy cookie secret, base64 (consumed by the proxy via the hubble-ui-sso-outputs Secret, never Git, never Proton Pass)"
  value       = random_bytes.cookie_secret.base64
  sensitive   = true
}
