variable "project_name" {
  description = "Zitadel project name and OIDC client name fallback (e.g. coder, hubble-ui)"
  type        = string
}

variable "group_name" {
  description = "Group claim value on the project roles (defaults to project_name; consumed by oauth2-proxy --allowed-group / app group gates)"
  type        = string
  default     = null
}

variable "oidc_name" {
  description = "Display name of the OIDC application (defaults to project_name)"
  type        = string
  default     = null
}

variable "domain" {
  description = "Zitadel external domain (issuer host, no scheme)"
  type        = string
  default     = "zitadel.home-ops.yansyah.my.id"
}

variable "jwt_profile_json" {
  description = "JWT profile key JSON for the FirstInstance IAM_OWNER machine user (controller injects via varsFrom from the ESO Kubernetes-provider mirror of zitadel-bootstrap-credentials; manual runs pass -var, never commit)"
  type        = string
  sensitive   = true
  default     = null
}

variable "org_id" {
  description = "Home-ops org ID from the FirstInstance handoff (zitadel-bootstrap-outputs Secret, mirrored via ESO into <app>-terraform-vars; controller injects via varsFrom)"
  type        = string
  default     = null
}

variable "admin_user_id" {
  description = "Bootstrap admin user ID from the FirstInstance handoff (zitadel-bootstrap-outputs Secret, mirrored via ESO into <app>-terraform-vars; controller injects via varsFrom) — always granted the admin project role"
  type        = string
  sensitive   = true
  default     = null
}

variable "admin_emails" {
  description = "Extra OIDC admins beyond the bootstrap admin: created as human users + granted the admin project role (empty = bootstrap admin only)"
  type        = list(string)
  default     = []
}

variable "redirect_uris" {
  description = "Fully-rendered OIDC redirect URIs (e.g. [\"https://coder.example.com/*\"] or [\"https://ui.example.com/oauth2/callback\"]) — callers render hosts, the root takes no app_host/ui_host vars"
  type        = list(string)
}

variable "post_logout_redirect_uris" {
  description = "Fully-rendered post-logout redirect URIs (e.g. [\"https://coder.example.com/\"])"
  type        = list(string)
}

variable "admin_role_key" {
  description = "Project role key granted to the bootstrap admin (defaults to <project_name>-admin)"
  type        = string
  default     = null
}

variable "create_user_role" {
  description = "Whether to create the non-admin project role (coder/headlamp pattern; admin-only proxies leave this false)"
  type        = bool
  default     = false
}

variable "user_role_key" {
  description = "Project role key granted to normal users (defaults to <project_name>-user; set create_user_role when user_emails is non-empty)"
  type        = string
  default     = null
}

variable "user_emails" {
  description = "Normal users to create + grant the user role (empty = admin-only)"
  type        = list(string)
  default     = []
}

variable "user_initial_password" {
  description = "Initial password for normal users (rotate after first login; shared across users in this slice)"
  type        = string
  sensitive   = true
  default     = null
}

variable "create_cookie_secret" {
  description = "Whether to generate a 32-byte oauth2-proxy cookie secret (hubble-ui/seaweedfs pattern; outputs as base64)"
  type        = bool
  default     = false
}
