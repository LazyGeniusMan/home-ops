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

variable "admin_initial_password" {
  description = "Initial admin password (rotate after first login)"
  type        = string
  sensitive   = true
}

variable "user_initial_password" {
  description = "Initial user password (rotate after first login)"
  type        = string
  sensitive   = true
}
