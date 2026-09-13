variable "domain" {
  description = "Zitadel external domain (issuer host, no scheme)"
  type        = string
  default     = "zitadel.home-ops.yansyah.my.id"
}

variable "app_host" {
  description = "Public Hubble UI hostname (no scheme; redirect https://<app-host>/* covers the proxy callback /oauth2/callback)"
  type        = string
  default     = "hubble.home-ops.yansyah.my.id"
}

variable "jwt_profile_json" {
  description = "JWT profile key JSON for the FirstInstance IAM_OWNER machine user (controller injects via varsFrom from the ESO Kubernetes-provider mirror of zitadel-bootstrap-credentials; manual runs pass -var, never commit)"
  type        = string
  sensitive   = true
  default     = null
}

variable "org_id" {
  description = "Home-ops org ID from the FirstInstance handoff (zitadel-bootstrap-outputs Secret, mirrored via ESO into hubble-ui-terraform-vars; controller injects via varsFrom)"
  type        = string
  default     = null
}

variable "admin_user_id" {
  description = "Bootstrap admin user ID from the FirstInstance handoff (zitadel-bootstrap-outputs Secret, mirrored via ESO into hubble-ui-terraform-vars; controller injects via varsFrom)"
  type        = string
  sensitive   = true
  default     = null
}
