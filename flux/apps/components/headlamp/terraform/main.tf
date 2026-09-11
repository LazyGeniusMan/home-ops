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

# Org anchor: the ID flows from the zitadel bootstrap slice via remote state
# (state Secret `tfstate-default-zitadel-bootstrap-identity` in the `zitadel`
# namespace, read with the in-cluster Kubernetes backend) — no literal org_id
# var, no manual per-env fill. The read runs as the headlamp-namespace tofu
# runner SA, whose narrow RBAC (Role + RoleBinding in the zitadel namespace,
# owned by this component in base/terraform-remote-state-rbac.yaml) grants
# get+list on the bootstrap state Secret only.
data "terraform_remote_state" "zitadel" {
  backend = "kubernetes"
  config = {
    secret_suffix     = "zitadel-bootstrap-identity"
    namespace         = "zitadel"
    in_cluster_config = true
  }
}

data "zitadel_org" "home_ops" {
  id = data.terraform_remote_state.zitadel.outputs.org_id
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

# Admin comes from the zitadel bootstrap remote-state output (stored ID — no
# email lookup needed); normal users are owned HERE (per-app decoupling —
# the bootstrap slice is admin-only). Empty user_emails = admin-only.
locals {
  admin_user_id = data.terraform_remote_state.zitadel.outputs.admin_user_id
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
