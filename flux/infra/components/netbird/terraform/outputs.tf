output "service_id" {
  description = "ID of the NetBird reverse-proxy service owned by this slice"
  value       = netbird_reverse_proxy_service.this.id
}

output "service_domain" {
  description = "Service FQDN the proxy answers on (echoes var.domain)"
  value       = netbird_reverse_proxy_service.this.domain
}

output "proxy_url" {
  description = "Public https URL for http-mode services (empty string for L4 modes, which listen on a dedicated port instead)"
  value       = var.mode == "http" ? "https://${var.domain}" : ""
}

output "proxy_cluster" {
  description = "Proxy cluster handling this service (derived from the domain)"
  value       = netbird_reverse_proxy_service.this.proxy_cluster
}

output "domain_id" {
  description = "ID of the custom-domain registration (empty string when create_custom_domain is false)"
  value       = var.create_custom_domain ? netbird_reverse_proxy_domain.this[0].id : ""
}

output "domain_validated" {
  description = "Whether the custom domain is validated (null when create_custom_domain is false — check the dashboard Verify step in README)"
  value       = var.create_custom_domain ? netbird_reverse_proxy_domain.this[0].validated : null
}

output "dns_record_name" {
  description = "Wildcard CNAME record name managing verification (empty string when no Cloudflare record is managed)"
  value       = var.create_custom_domain && var.cloudflare_zone_id != null ? cloudflare_dns_record.validation[0].name : ""
}

output "talos_setup_key" {
  description = "Plaintext reusable Talos setup key for var.cluster_name (unlimited uses, never expires; auto-joins the <cluster>-nodes group) — sensitive, lands in the consumer writeOutputsToSecret Secret, never in git"
  value       = netbird_setup_key.talos.key
  sensitive   = true
}

output "cluster_network_id" {
  description = "ID of the per-cluster NetBird network (var.cluster_name)"
  value       = netbird_network.cluster.id
}

output "cluster_nodes_group_id" {
  description = "ID of the per-cluster Talos nodes group (<cluster>-nodes, also the routing-peer group)"
  value       = netbird_group.cluster_nodes.id
}

output "service_lb_resource_id" {
  description = "ID of the Service Load Balancer IP network resource backing the shared reverse-proxy subnet target"
  value       = netbird_network_resource.service_lb.id
}

output "lan_resource_id" {
  description = "ID of the LAN CIDR network resource (admin-users path)"
  value       = netbird_network_resource.lan.id
}
