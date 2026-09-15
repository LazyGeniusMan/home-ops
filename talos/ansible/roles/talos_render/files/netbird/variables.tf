variable "netbird_token" {
  description = "NetBird management PAT (Ansible passes it ONLY as the NB_PAT env var, never as -var — the provider schema lets config win over env, so this stays null unless a manual run needs it)"
  type        = string
  sensitive   = true
  default     = null
}

variable "cluster_name" {
  description = "Talos cluster name owning this slice's NetBird fabric (e.g. acme-dev-bdo1-talos-apps-01) — parameterizes the per-cluster network, nodes group, and setup key names; Ansible passes talos_cluster per cluster"
  type        = string
}

variable "lan_cidr" {
  description = "LAN CIDR exposed to the admin-users group via the LAN network resource"
  type        = string
  default     = "192.168.1.0/24"
}

variable "management_url" {
  description = "NetBird management API URL (NetBird Cloud default)"
  type        = string
  default     = "https://api.netbird.io"
}
