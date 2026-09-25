variable "netbird_token" {
  description = "NetBird management PAT (Ansible passes it only as NB_PAT env, never -var)"
  type        = string
  sensitive   = true
  default     = null
}

variable "cluster_name" {
  description = "Talos cluster name owning this NetBird fabric (network, nodes group, setup key names)"
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
