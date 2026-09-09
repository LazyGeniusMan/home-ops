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

# Org anchor: the ID flows from the zitadel bootstrap slice via remote state
# (state Secret `tfstate-default-zitadel-bootstrap-identity` in the `zitadel`
# namespace, read with the in-cluster Kubernetes backend) — no literal org_id
# var, no manual per-env fill. The read runs as the hubble-ui-namespace tofu
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

# App project: the roles/grants below are scoped HERE, so `hubble-ui-admin`
# never implies org admin (project-scoped role only). project_role_check
# requires a grant to authenticate; project_role_assertion puts the roles in
# the token `groups` claim (consumed by the proxy --oidc-groups-claim).
resource "zitadel_project" "hubble_ui" {
  org_id                 = data.zitadel_org.home_ops.id
  name                   = "hubble-ui"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.hubble_ui.id
  role_key     = "hubble-ui-admin"
  display_name = "Hubble UI Admin"
  group        = "hubble-ui"
}

resource "zitadel_project_role" "user" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.hubble_ui.id
  role_key     = "hubble-ui-user"
  display_name = "Hubble UI User"
  group        = "hubble-ui"
}

# Users come from the zitadel bootstrap remote-state outputs (stored IDs —
# no email lookup needed). Admin (super-admin, ORG_OWNER) gets hubble-ui-admin;
# the normal user gets hubble-ui-user. The proxy gate stays admin-only
# (--allowed-group=hubble-ui-admin), so only the admin grant gates access
# today; the user role/grant is provisioned for the day the gate widens.
locals {
  admin_user_id = data.terraform_remote_state.zitadel.outputs.admin_user_id
  user_user_id  = data.terraform_remote_state.zitadel.outputs.user_user_id
}

resource "zitadel_user_grant" "admin" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.hubble_ui.id
  user_id    = local.admin_user_id
  role_keys  = ["hubble-ui-admin"]
}

resource "zitadel_user_grant" "user" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.hubble_ui.id
  user_id    = local.user_user_id
  role_keys  = ["hubble-ui-user"]
}

# oauth2-proxy cookie secret, generated in-Tofu (32 random bytes, base64 —
# mirrors `openssl rand -base64 32`). Stored ONLY in the outputs Secret, never
# Git, never Proton Pass; regenerated iff the state is recreated.
resource "random_bytes" "cookie_secret" {
  length = 32
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups). Relocated verbatim from the central zitadel module's `hubble` entry:
# name stays `hubble`, redirect https://<app-host>/* (covers the proxy callback
# /oauth2/callback), post-logout https://<app-host>/; app_host rides the
# per-env overlay CR vars. The generated client_id/client_secret are computed
# server-side — they land in the `hubble-ui-sso-outputs` Secret via the CR's
# writeOutputsToSecret and are consumed natively (k8s-provider ExternalSecret),
# never Git, never Proton Pass.
resource "zitadel_application_oidc" "hubble" {
  org_id                      = data.zitadel_org.home_ops.id
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
