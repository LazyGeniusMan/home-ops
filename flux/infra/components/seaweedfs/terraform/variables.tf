variable "domain" {
  description = "Zitadel external domain (issuer host, no scheme)"
  type        = string
  default     = "zitadel.home-ops.yansyah.my.id"
}

variable "org_id" {
  description = "home-ops org ID (plain literal per-env overlay value, non-sensitive; read from zitadel-bootstrap-outputs after bootstrap; empty falls back to org_name lookup)"
  type        = string
  default     = ""
}

variable "org_name" {
  description = "home-ops org display name (fallback lookup when org_id is empty)"
  type        = string
  default     = "home-ops"
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

variable "ui_host" {
  description = "Public SeaweedFS filer-UI hostname (no scheme; redirect https://<ui-host>/oauth2/callback)"
  type        = string
  default     = "ui.seaweedfs.home-ops.yansyah.my.id"
}

variable "jwt_profile_json" {
  description = "JWT profile key JSON for the IAM_OWNER service user (controller injects via varsFrom; manual runs pass -var, never commit)"
  type        = string
  sensitive   = true
  default     = null
}
