output "client_id" {
  description = "Generated OIDC client_id for the hubble client (seed into Proton Pass alongside the client secret from state)"
  value       = zitadel_application_oidc.hubble.client_id
  sensitive   = true
}
