output "client_id" {
  description = "Generated OIDC client_id for the coder client (seed into Proton Pass alongside the client secret from state)"
  value       = zitadel_application_oidc.coder.client_id
  sensitive   = true
}
