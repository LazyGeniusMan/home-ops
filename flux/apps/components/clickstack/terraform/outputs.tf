output "client_id" {
  description = "Generated OIDC client_id for the clickstack client (seed into Proton Pass alongside the client secret from state)"
  value       = zitadel_application_oidc.clickstack.client_id
  sensitive   = true
}
