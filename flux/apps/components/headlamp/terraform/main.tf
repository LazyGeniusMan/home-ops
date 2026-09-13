# Headlamp SSO identity (§13.3), machine-applied by the `headlamp-sso`
# Terraform CR (base/terraform.yaml, Tofu Controller) — owns ONLY this app's
# slice: its Zitadel project + project-scoped roles + user grants + OIDC
# client.
# Provider auth: the FirstInstance machine user (`zitadel-bootstrap-sa`,
# IAM_OWNER) via JWT profile — the controller injects var.jwt_profile_json from
# the ESO-synced `headlamp-terraform-vars` Secret (Kubernetes-provider mirror of
# the chart handoff `zitadel-bootstrap-credentials` in the `zitadel` namespace,
# never Git, never Proton Pass); for manual runs pass
# -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Org anchor: IDs flow from the FirstInstance handoff (`zitadel-bootstrap-outputs`
# Secret in the `zitadel` namespace: org_id + admin_user_id, operator-created once
# per the zitadel README runbook) via the ESO-synced `headlamp-terraform-vars`
# Secret (same-namespace `varsFrom` in base/terraform.yaml — a cross-namespace
# `headlamp-zitadel` SecretStore + the narrow `headlamp-zitadel-handoff-reader` Role in
# base/zitadel-handoff-rbac.yaml do the mirroring). No literal org_id in git, no
# manual per-env fill, no remote-state read off the retired bootstrap state.
data "zitadel_org" "home_ops" {
  id = var.org_id
}

# App project: the roles/grants below are scoped HERE, so `headlamp-admin`
# never implies org admin (project-scoped role only). project_role_check
# requires a grant to authenticate; project_role_assertion puts the roles in
# the token `groups` claim.
resource "zitadel_project" "headlamp" {
  org_id                 = data.zitadel_org.home_ops.id
  name                   = "headlamp"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.headlamp.id
  role_key     = "headlamp-admin"
  display_name = "Headlamp Admin"
  group        = "headlamp"
}

resource "zitadel_project_role" "user" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.headlamp.id
  role_key     = "headlamp-user"
  display_name = "Headlamp User"
  group        = "headlamp"
}

# Admin comes from the FirstInstance handoff var (stored ID — no
# email lookup needed); normal users are owned HERE (per-app decoupling —
# the bootstrap is admin-only). Empty user_emails = admin-only.
locals {
  admin_user_id = var.admin_user_id
}

resource "zitadel_user_grant" "admin" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.headlamp.id
  user_id    = local.admin_user_id
  role_keys  = ["headlamp-admin"]
}

# Normal users: plain org members (roles=[]) + headlamp-user grant.
# Headlamp-side RBAC distinguishes them from the admin (see the app README +
# base/headlamp-rbac.yaml).
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
  project_id = zitadel_project.headlamp.id
  user_id    = each.value.id
  role_keys  = ["headlamp-user"]
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups). Name stays `headlamp` (existing wiring references it); the generated
# client_id/client_secret are computed server-side — they flow out via the
# module outputs into the `headlamp-sso-outputs` Secret (CR
# writeOutputsToSecret), which the `headlamp-oidc` ExternalSecret consumes
# through the in-cluster `headlamp-k8s` SecretStore (no Proton Pass seeding).
# Redirect https://<app-host>/* covers the callback
# /oidc-callback; app_host rides the per-env overlay CR vars.
resource "zitadel_application_oidc" "headlamp" {
  org_id                      = data.zitadel_org.home_ops.id
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
