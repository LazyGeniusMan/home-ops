output "client_id" {
  description = "Generated OIDC client_id for the seaweedfs client (consumed by ui-auth via the outputs Secret, never Git, never Proton Pass)"
  value       = module.sso.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret for the seaweedfs client (consumed by ui-auth via the outputs Secret, never Git, never Proton Pass)"
  value       = module.sso.client_secret
  sensitive   = true
}

output "cookie_secret" {
  description = "Generated oauth2-proxy cookie secret, base64 (consumed by ui-auth via the outputs Secret, never Git, never Proton Pass)"
  value       = module.sso.cookie_secret
  sensitive   = true
}
