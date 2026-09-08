# SeaweedFS filer-UI SSO identity (§12 follow-up), machine-applied by the
# `seaweedfs-sso` Terraform CR (configs/base/terraform.yaml, Tofu Controller)
# — owns ONLY this component's slice: its Zitadel project + project-scoped
# roles + user grants + the `seaweedfs` OIDC client for the oauth2-proxy UI
# gate. The public S3 API route stays DIRECT (SigV4-gated machine endpoint,
# see the component README §12 decision) — no client needed for S3.
# Provider auth: a service user with IAM_OWNER (FirstInstance machine user) via
# JWT profile — the controller injects var.jwt_profile_json from the ESO-synced
# `seaweedfs-terraform-vars` Secret (Proton Pass, never Git); for manual runs
# pass -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Org anchor: name lookup with a literal-ID fallback. After the zitadel
# bootstrap first applies, read org_id from the `zitadel-bootstrap-outputs`
# Secret (zitadel namespace) into the per-env overlay CR vars (one-time
# manual step; the CR retries meanwhile). When org_id is set (non-empty), the
# literal `zitadel_org` lookup wins; otherwise the name search resolves the
# org created by the bootstrap (name "home-ops").
data "zitadel_orgs" "home_ops" {
  count       = var.org_id == "" ? 1 : 0
  name        = var.org_name
  name_method = "TEXT_QUERY_METHOD_EQUALS"
}

data "zitadel_org" "home_ops" {
  count = var.org_id == "" ? 0 : 1
  id    = var.org_id
}

locals {
  org_id = var.org_id == "" ? one(data.zitadel_orgs.home_ops[0].ids) : data.zitadel_org.home_ops[0].id
}

# Component project: the roles/grants below are scoped HERE, so
# `seaweedfs-admin` never implies org admin (project-scoped role only).
# project_role_check requires a grant to authenticate; project_role_assertion
# puts the roles in the token `groups` claim.
resource "zitadel_project" "seaweedfs" {
  org_id                 = local.org_id
  name                   = "seaweedfs"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = local.org_id
  project_id   = zitadel_project.seaweedfs.id
  role_key     = "seaweedfs-admin"
  display_name = "SeaweedFS Admin"
  group        = "seaweedfs"
}

resource "zitadel_project_role" "user" {
  org_id       = local.org_id
  project_id   = zitadel_project.seaweedfs.id
  role_key     = "seaweedfs-user"
  display_name = "SeaweedFS User"
  group        = "seaweedfs"
}

# Users resolved by email lookup (emails are plain vars); one() asserts exactly
# one match. Admin (super-admin) gets seaweedfs-admin; the normal user gets
# seaweedfs-user. The filer UI stays admin-only via the proxy's
# --allowed-group=seaweedfs-admin (see ui-auth.yaml); the user grant exists so
# a future read-only gate can bind it without touching the project.
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
  project_id = zitadel_project.seaweedfs.id
  user_id    = one(data.zitadel_human_users.admin.user_ids)
  role_keys  = ["seaweedfs-admin"]
}

resource "zitadel_user_grant" "user" {
  org_id     = local.org_id
  project_id = zitadel_project.seaweedfs.id
  user_id    = one(data.zitadel_human_users.user.user_ids)
  role_keys  = ["seaweedfs-user"]
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups). Name stays `seaweedfs` (matches the ui-auth wiring + Proton Pass
# paths below); the generated client_id/client_secret are computed
# server-side — read them from state (or the `seaweedfs-sso-outputs` Secret)
# after apply and seed pass://<env-vault>/seaweedfs/oauth2-proxy-client-id +
# oauth2-proxy-client-secret (never Git). Redirect
# https://<ui-host>/oauth2/callback serves the filer-UI proxy; ui_host rides
# the per-env overlay CR vars.
resource "zitadel_application_oidc" "seaweedfs" {
  org_id                      = local.org_id
  project_id                  = zitadel_project.seaweedfs.id
  name                        = "seaweedfs"
  redirect_uris               = ["https://${var.ui_host}/oauth2/callback"]
  post_logout_redirect_uris   = ["https://${var.ui_host}/"]
  response_types              = ["OIDC_RESPONSE_TYPE_CODE"]
  grant_types                 = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN"]
  app_type                    = "OIDC_APP_TYPE_WEB"
  auth_method_type            = "OIDC_AUTH_METHOD_TYPE_BASIC"
  access_token_type           = "OIDC_TOKEN_TYPE_BEARER"
  access_token_role_assertion = true
  id_token_role_assertion     = true
  id_token_userinfo_assertion = true
}
