# Identity-as-code for §11.1 (provider-ready, manually applied — see README
# runbook; no Tofu Controller exists in this repo). Single source of truth
# for the OIDC contract table in the component README. Provider auth: a
# service user with IAM_OWNER (FirstInstance machine user) via JWT profile —
# export ZITADEL_DOMAIN + the key JSON before running.
provider "zitadel" {
  domain = var.domain
}

resource "zitadel_org" "home_ops" {
  name = "home-ops"
}

resource "zitadel_human_user" "admin" {
  org_id            = zitadel_org.home_ops.id
  user_name         = var.admin_email
  first_name        = "Home-Ops"
  last_name         = "Admin"
  email             = var.admin_email
  is_email_verified = true
  initial_password  = var.admin_initial_password
}

resource "zitadel_human_user" "user" {
  org_id            = zitadel_org.home_ops.id
  user_name         = var.user_email
  first_name        = "Home-Ops"
  last_name         = "User"
  email             = var.user_email
  is_email_verified = true
  initial_password  = var.user_initial_password
}

# Groups: the pinned provider (zitadel ~> 3.3) ships no zitadel_user_group
# resources, so group membership is expressed with org/project membership:
# admin ≡ ORG_OWNER + project admin grant; users ≡ plain org member + project
# user grant. The `groups` claim on every client below still carries
# admin/users via the project role assertion + grants.
resource "zitadel_org_member" "admin_member" {
  org_id  = zitadel_org.home_ops.id
  user_id = zitadel_human_user.admin.id
  roles   = ["ORG_OWNER"]
}

resource "zitadel_org_member" "user_member" {
  org_id  = zitadel_org.home_ops.id
  user_id = zitadel_human_user.user.id
  roles   = []
}

# Roles/grants: home-ops project with admin/user roles; user grants bind each
# user to their role (drives id_token role assertion).
resource "zitadel_project" "home_ops" {
  org_id               = zitadel_org.home_ops.id
  name                 = "home-ops"
  project_role_check   = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id     = zitadel_org.home_ops.id
  project_id = zitadel_project.home_ops.id
  role_key   = "admin"
  display_name = "Admin"
  group      = "home-ops"
}

resource "zitadel_project_role" "user" {
  org_id     = zitadel_org.home_ops.id
  project_id = zitadel_project.home_ops.id
  role_key   = "user"
  display_name = "User"
  group      = "home-ops"
}

resource "zitadel_user_grant" "admin" {
  org_id     = zitadel_org.home_ops.id
  project_id = zitadel_project.home_ops.id
  user_id    = zitadel_human_user.admin.id
  role_keys  = ["admin"]
}

resource "zitadel_user_grant" "user" {
  org_id     = zitadel_org.home_ops.id
  project_id = zitadel_project.home_ops.id
  user_id    = zitadel_human_user.user.id
  role_keys  = ["user"]
}

# OIDC clients (all: code flow + PKCE, refresh tokens; scopes openid profile
# email groups). Client secrets are generated server-side — read them from
# state after apply and store in Proton Pass (never in Git).
locals {
  clients = {
    clickstack         = ["https://clickstack.home-ops.yansyah.my.id/*"]
    hubble             = ["https://hubble.home-ops.yansyah.my.id/*"]
    flux-operator-ui   = ["https://flux-operator.home-ops.yansyah.my.id/*"]
    headlamp           = ["https://headlamp.home-ops.yansyah.my.id/*"]
    coder              = ["https://coder.home-ops.yansyah.my.id/*"]
    oauth2-proxy-shared = ["https://*/oauth2/callback"]
  }
  post_logout = {
    clickstack         = ["https://clickstack.home-ops.yansyah.my.id/"]
    hubble             = ["https://hubble.home-ops.yansyah.my.id/"]
    flux-operator-ui   = ["https://flux-operator.home-ops.yansyah.my.id/"]
    headlamp           = ["https://headlamp.home-ops.yansyah.my.id/"]
    coder              = ["https://coder.home-ops.yansyah.my.id/"]
    oauth2-proxy-shared = []
  }
}

resource "zitadel_application_oidc" "clients" {
  for_each                     = local.clients
  org_id                       = zitadel_org.home_ops.id
  project_id                   = zitadel_project.home_ops.id
  name                         = each.key
  redirect_uris                = each.value
  post_logout_redirect_uris    = local.post_logout[each.key]
  response_types               = ["OIDC_RESPONSE_TYPE_CODE"]
  grant_types                  = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN"]
  app_type                     = "OIDC_APP_TYPE_WEB"
  auth_method_type             = "OIDC_AUTH_METHOD_TYPE_BASIC"
  access_token_type            = "OIDC_TOKEN_TYPE_BEARER"
  access_token_role_assertion  = true
  id_token_role_assertion      = true
  id_token_userinfo_assertion  = true
}
