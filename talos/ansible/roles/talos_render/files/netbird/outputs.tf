output "talos_setup_key" {
  description = "Reusable Talos setup key for var.cluster_name — sensitive, Ansible-only"
  value       = netbird_setup_key.talos.key
  sensitive   = true
}

output "cluster_network_id" {
  description = "Per-cluster NetBird network ID (var.cluster_name)"
  value       = netbird_network.cluster.id
}

output "cluster_nodes_group_id" {
  description = "Per-cluster Talos nodes group ID (also the routing-peer group)"
  value       = netbird_group.cluster_nodes.id
}

output "lan_resource_id" {
  description = "LAN CIDR network resource ID (admin-users path)"
  value       = netbird_network_resource.lan.id
}
