# No netbird_token variable by design: the provider reads NB_PAT straight from
# the environment (Ansible sets it from `pass-cli item view`, never -var,
# never on disk, never in state).
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
