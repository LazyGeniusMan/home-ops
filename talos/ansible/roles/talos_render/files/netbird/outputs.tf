output "talos_setup_key" {
  description = "Plaintext reusable Talos setup key for var.cluster_name (unlimited uses, never expires; auto-joins the <cluster>-nodes group) — sensitive, consumed by Ansible only (never committed, never logged)"
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

output "lan_resource_id" {
  description = "ID of the LAN CIDR network resource (admin-users path)"
  value       = netbird_network_resource.lan.id
}
