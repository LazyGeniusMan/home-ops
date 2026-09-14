output "client_id" {
  description = "Generated OIDC client_id (lands in the <app>-sso-outputs Secret via the CR writeOutputsToSecret; consumed by the app ExternalSecret via the <app>-k8s SecretStore)"
  value       = zitadel_application_oidc.this.client_id
  sensitive   = true
}

output "client_secret" {
  description = "Generated OIDC client_secret (lands in the <app>-sso-outputs Secret via the CR writeOutputsToSecret; consumed by the app ExternalSecret via the <app>-k8s SecretStore)"
  value       = zitadel_application_oidc.this.client_secret
  sensitive   = true
}

output "project_id" {
  description = "ID of the Zitadel project owned by this slice"
  value       = zitadel_project.this.id
}

output "cookie_secret" {
  description = "Generated oauth2-proxy cookie secret, base64 (empty string when create_cookie_secret is false; consumed by the proxy via the <app>-sso-outputs Secret, never Git, never Proton Pass)"
  value       = var.create_cookie_secret ? random_bytes.cookie_secret[0].base64 : ""
  sensitive   = true
}

output "admin_role_key" {
  description = "Effective admin project role key granted to the bootstrap admin"
  value       = local.admin_role_key
}

output "user_role_key" {
  description = "Effective user project role key granted to normal users"
  value       = local.user_role_key
}
