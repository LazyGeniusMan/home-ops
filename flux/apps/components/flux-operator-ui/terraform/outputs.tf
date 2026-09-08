output "client_id" {
  description = "Generated OIDC client_id for the flux-operator-ui client (seed into Proton Pass alongside the client secret from state)"
  value       = zitadel_application_oidc.flux_operator_ui.client_id
  sensitive   = true
}
