# Canonical Zitadel SSO slice: one Zitadel project + project-scoped roles +
# user grants + OIDC client (code flow + PKCE, refresh tokens). Consumed via a
# thin per-app root (see README.md) because Tofu Controller sources are
# per-component OCI artifacts that cannot `path:` into flux/infra directly.
#
# Roles/grants are scoped to THIS project, so `<app>-admin` never implies org
# admin. project_role_check requires a grant to authenticate;
# project_role_assertion puts the roles in the token `groups` claim.
locals {
  group_name     = coalesce(var.group_name, var.project_name)
  oidc_name      = coalesce(var.oidc_name, var.project_name)
  admin_role_key = coalesce(var.admin_role_key, "${var.project_name}-admin")
  user_role_key  = coalesce(var.user_role_key, "${var.project_name}-user")
  # Title-case display names mirror the consumer convention ("Coder Admin").
  display_prefix = join(" ", [for w in split("-", var.project_name) : "${upper(substr(w, 0, 1))}${substr(w, 1, -1)}"])
}

# Org anchor: IDs flow from the FirstInstance handoff (`zitadel-bootstrap-outputs`
# Secret in the `zitadel` namespace: org_id + admin_user_id, operator-created once
# per the zitadel README runbook) via the ESO-synced `<app>-terraform-vars`
# Secret (same-namespace `varsFrom` in the consumer's base/terraform.yaml).
# No literal org_id in git, no manual per-env fill, no remote-state read off
# the retired bootstrap state.
data "zitadel_org" "home_ops" {
  id = var.org_id
}

resource "zitadel_project" "this" {
  org_id                 = data.zitadel_org.home_ops.id
  name                   = var.project_name
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.this.id
  role_key     = local.admin_role_key
  display_name = "${local.display_prefix} Admin"
  group        = local.group_name
}

resource "zitadel_project_role" "user" {
  count        = var.create_user_role ? 1 : 0
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.this.id
  role_key     = local.user_role_key
  display_name = "${local.display_prefix} User"
  group        = local.group_name
}

# Admin comes from the FirstInstance handoff var (stored ID — no email lookup
# needed); normal users are owned HERE (per-app decoupling — the bootstrap is
# admin-only). Empty user_emails = admin-only.
resource "zitadel_user_grant" "admin" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.this.id
  user_id    = var.admin_user_id
  role_keys  = [local.admin_role_key]
}

# Extra OIDC admins beyond the bootstrap admin (admin_emails): created as
# human users here + granted the admin project role. Empty = bootstrap admin
# only. Shares user_initial_password (null = invite/reset flow, never git).
resource "zitadel_human_user" "admins" {
  for_each          = toset(var.admin_emails)
  org_id            = data.zitadel_org.home_ops.id
  user_name         = each.value
  first_name        = "Home-Ops"
  last_name         = "Admin"
  email             = each.value
  is_email_verified = true
  initial_password  = var.user_initial_password
}

resource "zitadel_org_member" "admins" {
  for_each = zitadel_human_user.admins
  org_id   = data.zitadel_org.home_ops.id
  user_id  = each.value.id
  roles    = []
}

resource "zitadel_user_grant" "extra_admins" {
  for_each   = zitadel_human_user.admins
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.this.id
  user_id    = each.value.id
  role_keys  = [local.admin_role_key]
}

# Normal users: plain org members (roles=[]) + user-role grant. App-side
# ownership/RBAC distinguishes them from the admin (see the app README).
resource "zitadel_human_user" "users" {
  for_each          = toset(var.user_emails)
  org_id            = data.zitadel_org.home_ops.id
  user_name         = each.value
  first_name        = "Home-Ops"
  last_name         = "User"
  email             = each.value
  is_email_verified = true
  initial_password  = var.user_initial_password
}

resource "zitadel_org_member" "users" {
  for_each = zitadel_human_user.users
  org_id   = data.zitadel_org.home_ops.id
  user_id  = each.value.id
  roles    = []
}

resource "zitadel_user_grant" "users" {
  for_each   = zitadel_human_user.users
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.this.id
  user_id    = each.value.id
  role_keys  = [local.user_role_key]
}

# oauth2-proxy cookie secret, generated in-Tofu (32 random bytes, base64 —
# mirrors `openssl rand -base64 32`). Stored ONLY in the outputs Secret, never
# Git, never Proton Pass; regenerated iff the state is recreated.
resource "random_bytes" "cookie_secret" {
  count  = var.create_cookie_secret ? 1 : 0
  length = 32
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups). The generated client_id/client_secret are computed server-side —
# they flow out via the module outputs into the `<app>-sso-outputs` Secret (CR
# writeOutputsToSecret), consumed by the app's ExternalSecret through the
# in-cluster `<app>-k8s` SecretStore (no Proton Pass seeding).
resource "zitadel_application_oidc" "this" {
  org_id                      = data.zitadel_org.home_ops.id
  project_id                  = zitadel_project.this.id
  name                        = local.oidc_name
  redirect_uris               = var.redirect_uris
  post_logout_redirect_uris   = var.post_logout_redirect_uris
  response_types              = ["OIDC_RESPONSE_TYPE_CODE"]
  grant_types                 = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN"]
  app_type                    = "OIDC_APP_TYPE_WEB"
  auth_method_type            = "OIDC_AUTH_METHOD_TYPE_BASIC"
  access_token_type           = "OIDC_TOKEN_TYPE_BEARER"
  access_token_role_assertion = true
  id_token_role_assertion     = true
  id_token_userinfo_assertion = true
}
