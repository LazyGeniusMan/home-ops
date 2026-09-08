variable "domain" {
  description = "Zitadel external domain (issuer host, no scheme)"
  type        = string
  default     = "zitadel.home-ops.yansyah.my.id"
}

variable "org_name" {
  description = "home-ops org name for the name-lookup fallback (used only while org_id is unset)"
  type        = string
  default     = "home-ops"
}

variable "org_id" {
  description = "home-ops org ID (plain literal per-env overlay value, non-sensitive; read from zitadel-bootstrap-outputs after bootstrap; wins over the org_name lookup when set)"
  type        = string
  default     = null
}

variable "admin_email" {
  description = "Super-admin human user email"
  type        = string
  default     = "admin@home-ops.yansyah.my.id"
}

variable "user_email" {
  description = "Normal member human user email"
  type        = string
  default     = "user@home-ops.yansyah.my.id"
}

variable "app_host" {
  description = "Public Flux Operator UI hostname (no scheme; redirect https://<app-host>/*)"
  type        = string
  default     = "flux-operator.home-ops.yansyah.my.id"
}

variable "jwt_profile_json" {
  description = "JWT profile key JSON for the IAM_OWNER service user (controller injects via varsFrom; manual runs pass -var, never commit)"
  type        = string
  sensitive   = true
  default     = null
}
