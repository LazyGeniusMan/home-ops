output "client_id" {
  description = "Generated OIDC client_id for the seaweedfs client (seed into Proton Pass alongside the client secret from state)"
  value       = zitadel_application_oidc.seaweedfs.client_id
  sensitive   = true
}
