output "talos_setup_key" {
  description = "Reusable Talos setup key for var.cluster_name — sensitive, Ansible-only"
  value       = netbird_setup_key.talos.key
  sensitive   = true
}

# Audit outputs for the rotation cadence (RUNBOOK §1.0b): watch expires before
# the 90d mark and used_times/last_used for unexpected peer joins.
output "setup_key_expires" {
  description = "Absolute expiry date of the Talos setup key (rotate well before)"
  value       = netbird_setup_key.talos.expires
}

output "setup_key_used_times" {
  description = "How often the Talos setup key has minted a peer (unexpected growth = investigate)"
  value       = netbird_setup_key.talos.used_times
}

output "setup_key_last_used" {
  description = "Last usage time of the Talos setup key"
  value       = netbird_setup_key.talos.last_used
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
