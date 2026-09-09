variable "domain" {
  description = "Zitadel external domain (issuer host, no scheme)"
  type        = string
  default     = "zitadel.home-ops.yansyah.my.id"
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
  description = "Public Hubble UI hostname (no scheme; redirect https://<app-host>/* covers the proxy callback /oauth2/callback)"
  type        = string
  default     = "hubble.home-ops.yansyah.my.id"
}

variable "jwt_profile_json" {
  description = "JWT profile key JSON for the IAM_OWNER service user (controller injects via varsFrom; manual runs pass -var, never commit)"
  type        = string
  sensitive   = true
  default     = null
}
