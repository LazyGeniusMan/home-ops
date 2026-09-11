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

# Org anchor: the ID flows from the zitadel bootstrap slice via remote state
# (state Secret `tfstate-default-zitadel-bootstrap-identity` in the `zitadel`
# namespace, read with the in-cluster Kubernetes backend) — no literal org_id
# var, no manual per-env fill. The read runs as the flux-operator-ui-namespace
# tofu runner SA, whose narrow RBAC (Role + RoleBinding in the zitadel
# namespace, owned by this component in base/terraform-remote-state-rbac.yaml)
# grants get+list on the bootstrap state Secret only.
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

# App project: the roles/grants below are scoped HERE, so
# `flux-operator-ui-admin` never implies org admin (project-scoped role only).
# project_role_check requires a grant to authenticate; project_role_assertion
# puts the roles in the token `groups` claim.
resource "zitadel_project" "flux_operator_ui" {
  org_id                 = data.zitadel_org.home_ops.id
  name                   = "flux-operator-ui"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.flux_operator_ui.id
  role_key     = "flux-operator-ui-admin"
  display_name = "Flux Operator UI Admin"
  group        = "flux-operator-ui"
}

resource "zitadel_project_role" "user" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.flux_operator_ui.id
  role_key     = "flux-operator-ui-user"
  display_name = "Flux Operator UI User"
  group        = "flux-operator-ui"
}

# Admin user comes from the zitadel bootstrap remote-state output (stored ID —
# no email lookup needed). Admin (super-admin, ORG_OWNER) gets
# flux-operator-ui-admin. Only the admin role passes the oauth2-proxy
# `--allowed-group` gate (admin-only UI, no user grant).
locals {
  admin_user_id = data.terraform_remote_state.zitadel.outputs.admin_user_id
}

resource "zitadel_user_grant" "admin" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.flux_operator_ui.id
  user_id    = local.admin_user_id
  role_keys  = ["flux-operator-ui-admin"]
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups) — relocated verbatim from the central zitadel module's
# `flux-operator-ui` for_each entry (redirect https://<app-host>/* covers
# /oauth2/callback, post-logout https://<app-host>/). Name stays
# `flux-operator-ui`; the generated client_id/client_secret are computed
# server-side — they flow out via the module outputs into the
# `flux-operator-ui-sso-outputs` Secret (CR writeOutputsToSecret), which the
# `oauth2-proxy-credentials` ExternalSecret consumes through the in-cluster
# `flux-operator-ui-k8s` SecretStore (no Proton Pass seeding for OIDC creds).
# app_host rides the per-env overlay CR vars.
resource "zitadel_application_oidc" "flux_operator_ui" {
  org_id                      = data.zitadel_org.home_ops.id
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
