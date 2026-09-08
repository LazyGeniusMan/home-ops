# Hubble-ui SSO identity (§11.3), machine-applied by the `hubble-ui-sso`
# Terraform CR (base/terraform.yaml, Tofu Controller) — owns ONLY this app's
# slice: its Zitadel project + project-scoped roles + user grants + OIDC
# client.
# Provider auth: a service user with IAM_OWNER (FirstInstance machine user) via
# JWT profile — the controller injects var.jwt_profile_json from the ESO-synced
# `hubble-ui-terraform-vars` Secret (Proton Pass, never Git); for manual runs
# pass -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Org anchor: name lookup with a literal fallback. data.zitadel_orgs filters by
# name ("home-ops"); var.org_id (plain literal per-env overlay value,
# non-sensitive) wins when set — read it from the `zitadel-bootstrap-outputs`
# Secret (zitadel namespace) after the bootstrap first applies (one-time manual
# step; the CR retries meanwhile). When empty, the lookup result is used.
data "zitadel_orgs" "home_ops" {
  name        = var.org_name
  name_method = "TEXT_QUERY_METHOD_EQUALS"
}

locals {
  org_id = var.org_id != "" ? var.org_id : one(data.zitadel_orgs.home_ops.ids)
}

# App project: the roles/grants below are scoped HERE, so `hubble-ui-admin`
# never implies org admin (project-scoped role only). project_role_check
# requires a grant to authenticate; project_role_assertion puts the roles in
# the token `groups` claim (consumed by the proxy --oidc-groups-claim).
resource "zitadel_project" "hubble_ui" {
  org_id                 = local.org_id
  name                   = "hubble-ui"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = local.org_id
  project_id   = zitadel_project.hubble_ui.id
  role_key     = "hubble-ui-admin"
  display_name = "Hubble UI Admin"
  group        = "hubble-ui"
}

resource "zitadel_project_role" "user" {
  org_id       = local.org_id
  project_id   = zitadel_project.hubble_ui.id
  role_key     = "hubble-ui-user"
  display_name = "Hubble UI User"
  group        = "hubble-ui"
}

# Users resolved by email lookup (emails are plain vars); one() asserts exactly
# one match. Admin (super-admin, ORG_OWNER) gets hubble-ui-admin; the normal
# user gets hubble-ui-user. The proxy gate stays admin-only
# (--allowed-group=hubble-ui-admin), so only the admin grant gates access
# today; the user role/grant is provisioned for the day the gate widens.
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
  project_id = zitadel_project.hubble_ui.id
  user_id    = one(data.zitadel_human_users.admin.user_ids)
  role_keys  = ["hubble-ui-admin"]
}

resource "zitadel_user_grant" "user" {
  org_id     = local.org_id
  project_id = zitadel_project.hubble_ui.id
  user_id    = one(data.zitadel_human_users.user.user_ids)
  role_keys  = ["hubble-ui-user"]
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups). Relocated verbatim from the central zitadel module's `hubble` entry:
# name stays `hubble`, redirect https://<app-host>/* (covers the proxy callback
# /oauth2/callback), post-logout https://<app-host>/; app_host rides the
# per-env overlay CR vars. The generated client_id/client_secret are computed
# server-side — read them from state (or the `hubble-ui-sso-outputs` Secret)
# after apply and seed pass://<env-vault>/hubble-ui/oidc-client-id (+ overwrite
# pass://<env-vault>/hubble-ui/oauth2-proxy-client-secret) — never Git.
resource "zitadel_application_oidc" "hubble" {
  org_id                      = local.org_id
  project_id                  = zitadel_project.hubble_ui.id
  name                        = "hubble"
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
