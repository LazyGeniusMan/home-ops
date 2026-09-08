output "client_id" {
  description = "Generated OIDC client_id for the headlamp client (seed into Proton Pass alongside the client secret from state)"
  value       = zitadel_application_oidc.headlamp.client_id
  sensitive   = true
}
