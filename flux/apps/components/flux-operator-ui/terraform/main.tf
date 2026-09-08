# Flux Operator UI SSO identity (§11.4), machine-applied by the
# `flux-operator-ui-sso` Terraform CR (base/terraform.yaml, Tofu Controller)
# — owns ONLY this app's slice: its Zitadel project + project-scoped roles +
# user grants + OIDC client.
# Provider auth: a service user with IAM_OWNER (FirstInstance machine user) via
# JWT profile — the controller injects var.jwt_profile_json from the ESO-synced
# `flux-operator-ui-terraform-vars` Secret (Proton Pass, never Git); for manual
# runs pass -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Org anchor: name lookup with a literal-ID fallback. After the zitadel
# bootstrap first applies, read org_id from the `zitadel-bootstrap-outputs`
# Secret (zitadel namespace) into the per-env overlay CR vars (one-time manual
# step; the CR retries meanwhile). Until org_id is filled, the name lookup
# resolves the org instead (count-gated so a failing lookup can never wedge a
# plan that already carries the literal ID).
data "zitadel_orgs" "home_ops" {
  count       = var.org_id == null ? 1 : 0
  name        = var.org_name
  name_method = "TEXT_QUERY_METHOD_EQUALS"
  state       = "ORG_STATE_ACTIVE"
}

locals {
  # Literal per-env org_id wins when set; otherwise the single name-lookup
  # hit (one() asserts exactly one match).
  org_id = var.org_id != null ? var.org_id : one(data.zitadel_orgs.home_ops[0].ids)
}

# App project: the roles/grants below are scoped HERE, so
# `flux-operator-ui-admin` never implies org admin (project-scoped role only).
# project_role_check requires a grant to authenticate; project_role_assertion
# puts the roles in the token `groups` claim.
resource "zitadel_project" "flux_operator_ui" {
  org_id                 = local.org_id
  name                   = "flux-operator-ui"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = local.org_id
  project_id   = zitadel_project.flux_operator_ui.id
  role_key     = "flux-operator-ui-admin"
  display_name = "Flux Operator UI Admin"
  group        = "flux-operator-ui"
}

resource "zitadel_project_role" "user" {
  org_id       = local.org_id
  project_id   = zitadel_project.flux_operator_ui.id
  role_key     = "flux-operator-ui-user"
  display_name = "Flux Operator UI User"
  group        = "flux-operator-ui"
}

# Users resolved by email lookup (emails are plain vars); one() asserts exactly
# one match. Admin (super-admin, ORG_OWNER) gets flux-operator-ui-admin; the
# normal user gets flux-operator-ui-user. Only the admin role passes the
# oauth2-proxy `--allowed-group` gate (admin-only UI stays admin-only).
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
  project_id = zitadel_project.flux_operator_ui.id
  user_id    = one(data.zitadel_human_users.admin.user_ids)
  role_keys  = ["flux-operator-ui-admin"]
}

resource "zitadel_user_grant" "user" {
  org_id     = local.org_id
  project_id = zitadel_project.flux_operator_ui.id
  user_id    = one(data.zitadel_human_users.user.user_ids)
  role_keys  = ["flux-operator-ui-user"]
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups) — relocated verbatim from the central zitadel module's
# `flux-operator-ui` for_each entry (redirect https://<app-host>/* covers
# /oauth2/callback, post-logout https://<app-host>/). Name stays
# `flux-operator-ui`; the generated client_id/client_secret are computed
# server-side — read them from state (or the `flux-operator-ui-sso-outputs`
# Secret) after apply and seed
# pass://<env-vault>/flux-operator-ui/oauth2-proxy-client-id +
# oauth2-proxy-client-secret (never Git). app_host rides the per-env overlay CR
# vars.
resource "zitadel_application_oidc" "flux_operator_ui" {
  org_id                      = local.org_id
  project_id                  = zitadel_project.flux_operator_ui.id
  name                        = "flux-operator-ui"
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
