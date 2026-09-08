# Identity-as-code for §11.1, machine-applied by the
# `zitadel-bootstrap-identity` Terraform CR (configs/base, Tofu Controller) —
# the single source of truth for the OIDC contract table in the component
# README. Provider auth: a service user with IAM_OWNER (FirstInstance machine
# user) via JWT profile — the controller injects var.jwt_profile_json from the
# ESO-synced `zitadel-terraform-vars` Secret (Proton Pass, never Git); for
# manual runs pass -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
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
# Parent domain derived from var.domain (issuer host zitadel.<parent>).
# Passing -var domain=zitadel.homelab-dev.yansyah.my.id (+ admin/user emails)
# switches every redirect to dev with no other edits (§14).
locals {
  parent_domain = replace(var.domain, "/^zitadel\\./", "")
  clients = {
    clickstack         = ["https://clickstack.${local.parent_domain}/*"]
    hubble             = ["https://hubble.${local.parent_domain}/*"]
    flux-operator-ui   = ["https://flux-operator.${local.parent_domain}/*"]
    headlamp           = ["https://headlamp.${local.parent_domain}/*"]
    coder              = ["https://coder.${local.parent_domain}/*"]
    oauth2-proxy-shared = ["https://*/oauth2/callback"]
  }
  post_logout = {
    clickstack         = ["https://clickstack.${local.parent_domain}/"]
    hubble             = ["https://hubble.${local.parent_domain}/"]
    flux-operator-ui   = ["https://flux-operator.${local.parent_domain}/"]
    headlamp           = ["https://headlamp.${local.parent_domain}/"]
    coder              = ["https://coder.${local.parent_domain}/"]
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
