# Clickstack SSO identity (§11.2), machine-applied by the `clickstack-sso`
# Terraform CR (base/terraform.yaml, Tofu Controller) — owns ONLY this app's
# slice: its Zitadel project + project-scoped roles + user grants + OIDC
# client. Provider auth: a service user with IAM_OWNER (FirstInstance machine
# user) via JWT profile — the controller injects var.jwt_profile_json from the
# ESO-synced `clickstack-terraform-vars` Secret (Proton Pass, never Git); for
# manual runs pass -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Org anchor: literal ID (plain var, non-sensitive, no ESO needed). After the
# zitadel bootstrap first applies, read org_id from the
# `zitadel-bootstrap-outputs` Secret (zitadel namespace) into the per-env
# overlay CR vars (one-time manual step; the CR retries meanwhile).
data "zitadel_org" "home_ops" {
  id = var.org_id
}

# App project: the roles/grants below are scoped HERE, so `clickstack-admin`
# never implies org admin (project-scoped role only). project_role_check
# requires a grant to authenticate; project_role_assertion puts the roles in
# the token `groups` claim.
resource "zitadel_project" "clickstack" {
  org_id                 = data.zitadel_org.home_ops.id
  name                   = "clickstack"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.clickstack.id
  role_key     = "clickstack-admin"
  display_name = "Clickstack Admin"
  group        = "clickstack"
}

resource "zitadel_project_role" "user" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.clickstack.id
  role_key     = "clickstack-user"
  display_name = "Clickstack User"
  group        = "clickstack"
}

# Users resolved by email lookup (emails are plain vars); one() asserts exactly
# one match. Admin (super-admin, ORG_OWNER) gets clickstack-admin; the normal
# user gets clickstack-user. Only clickstack-admin may sign in — the per-app
# oauth2-proxy sidecar gates on `--allowed-group=clickstack-admin`.
data "zitadel_human_users" "admin" {
  org_id       = data.zitadel_org.home_ops.id
  email        = var.admin_email
  email_method = "TEXT_QUERY_METHOD_EQUALS"
}

data "zitadel_human_users" "user" {
  org_id       = data.zitadel_org.home_ops.id
  email        = var.user_email
  email_method = "TEXT_QUERY_METHOD_EQUALS"
}

resource "zitadel_user_grant" "admin" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.clickstack.id
  user_id    = one(data.zitadel_human_users.admin.user_ids)
  role_keys  = ["clickstack-admin"]
}

resource "zitadel_user_grant" "user" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.clickstack.id
  user_id    = one(data.zitadel_human_users.user.user_ids)
  role_keys  = ["clickstack-user"]
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups). Relocated verbatim from the `clickstack` entry of the central
# zitadel module's client for_each (name + wildcard redirect
# https://<app-host>/* covers the proxy callback /oauth2/callback — same as
# the shared client `https://*/oauth2/callback` covered). app_host rides the
# per-env overlay CR vars. The generated client_id/client_secret are computed
# server-side — read them from state (or the `clickstack-sso-outputs` Secret)
# after apply and seed pass://<env-vault>/clickstack/oauth2-proxy-client-id +
# oauth2-proxy-client-secret (never Git).
resource "zitadel_application_oidc" "clickstack" {
  org_id                      = data.zitadel_org.home_ops.id
  project_id                  = zitadel_project.clickstack.id
  name                        = "clickstack"
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
