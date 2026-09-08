# Headlamp SSO identity (§13.3), machine-applied by the `headlamp-sso`
# Terraform CR (base/terraform.yaml, Tofu Controller) — owns ONLY this app's
# slice: its Zitadel project + project-scoped roles + user grants + OIDC
# client.
# Provider auth: a service user with IAM_OWNER (FirstInstance machine user) via
# JWT profile — the controller injects var.jwt_profile_json from the ESO-synced
# `headlamp-terraform-vars` Secret (Proton Pass, never Git); for manual runs pass
# -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Org anchor: preferred name lookup for `home-ops` (exact match); the literal
# var.org_id wins when set (one-time fill from `zitadel-bootstrap-outputs`,
# zitadel namespace; non-sensitive). Manual runs with no -var org_id resolve
# via lookup — no bootstrap-outputs read needed.
data "zitadel_orgs" "home_ops" {
  count       = var.org_id == "" ? 1 : 0
  name        = "home-ops"
  name_method = "TEXT_QUERY_METHOD_EQUALS"
}

locals {
  org_id = var.org_id != "" ? var.org_id : one(data.zitadel_orgs.home_ops[0].ids)
}

# App project: the roles/grants below are scoped HERE, so `headlamp-admin`
# never implies org admin (project-scoped role only). project_role_check
# requires a grant to authenticate; project_role_assertion puts the roles in
# the token `groups` claim.
resource "zitadel_project" "headlamp" {
  org_id                 = local.org_id
  name                   = "headlamp"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = local.org_id
  project_id   = zitadel_project.headlamp.id
  role_key     = "headlamp-admin"
  display_name = "Headlamp Admin"
  group        = "headlamp"
}

resource "zitadel_project_role" "user" {
  org_id       = local.org_id
  project_id   = zitadel_project.headlamp.id
  role_key     = "headlamp-user"
  display_name = "Headlamp User"
  group        = "headlamp"
}

# Users resolved by email lookup (emails are plain vars); one() asserts exactly
# one match. Admin (super-admin, ORG_OWNER) gets headlamp-admin; the normal user
# gets headlamp-user. Both groups sign in; Headlamp-side RBAC distinguishes
# them (see the app README + base/headlamp-rbac.yaml).
data "zitadel_human_users" "admin" {
  org_id       = local.org_id
  email        = var.admin_email
  email_method = "TEXT_QUERY_METHOD_EQUALS"
}

data "zitadel_human_users" "user" {
  org_id       = local.org_id
  email        = var.user_email
  email_method = "TEXT_QUERY_METHOD_EQUALS"
}

resource "zitadel_user_grant" "admin" {
  org_id     = local.org_id
  project_id = zitadel_project.headlamp.id
  user_id    = one(data.zitadel_human_users.admin.user_ids)
  role_keys  = ["headlamp-admin"]
}

resource "zitadel_user_grant" "user" {
  org_id     = local.org_id
  project_id = zitadel_project.headlamp.id
  user_id    = one(data.zitadel_human_users.user.user_ids)
  role_keys  = ["headlamp-user"]
}

# OIDC client (relocated from the `headlamp` entry of the central zitadel
# module's client for_each — same name, same redirect shape, same code flow +
# assertions). Name stays `headlamp` (existing wiring references it); the
# generated client_id/client_secret are computed server-side — read them from
# state (or the `headlamp-sso-outputs` Secret) after apply and seed
# pass://<env-vault>/headlamp/oidc-client-id + oidc-client-secret (never Git).
# Redirect https://<app-host>/* covers the callback /oidc-callback; app_host
# rides the per-env overlay CR vars.
resource "zitadel_application_oidc" "headlamp" {
  org_id                      = local.org_id
  project_id                  = zitadel_project.headlamp.id
  name                        = "headlamp"
  redirect_uris               = ["https://${var.app_host}/*"]
  post_logout_redirect_uris   = ["https://${var.app_host}/"]
  response_types              = ["OIDC_RESPONSE_TYPE_CODE"]
  grant_types                 = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN"]
  app_type                    = "OIDC_APP_TYPE_WEB"
  auth_method_type            = "OIDC_AUTH_METHOD_TYPE_BASIC"
  access_token_type           = "OIDC_TOKEN_TYPE_BEARER"
  access_token_role_assertion = true
  id_token_role_assertion     = true
  id_token_userinfo_assertion = true
}
