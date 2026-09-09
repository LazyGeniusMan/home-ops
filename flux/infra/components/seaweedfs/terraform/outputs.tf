output "client_id" {
  description = "Generated OIDC client_id for the seaweedfs client (consumed by ui-auth via the outputs Secret, never Git, never Proton Pass)"
  value       = zitadel_application_oidc.seaweedfs.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the seaweedfs client (consumed by ui-auth via the outputs Secret, never Git, never Proton Pass)"
  value       = zitadel_application_oidc.seaweedfs.client_secret
  sensitive   = true
}

output "cookie_secret" {
  description = "Generated oauth2-proxy cookie secret, base64 (consumed by ui-auth via the outputs Secret, never Git, never Proton Pass)"
  value       = random_bytes.cookie_secret.base64
  sensitive   = true
}
