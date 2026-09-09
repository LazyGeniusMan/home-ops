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

# Org + user anchor: IDs come from the zitadel bootstrap state
# (`tfstate-default-zitadel-bootstrap-identity`, zitadel namespace) via the
# in-cluster kubernetes backend — no org_id literal, no email lookups, no
# ESO/pass:// for any ID. The tf-runner reads that Secret through the narrow
# Role/RoleBinding shipped in configs/base/rbac.yaml
# (system:serviceaccount:seaweedfs:tf-runner → get on that Secret only).
# Own state keeps the default backend (state Secrets in the seaweedfs
# namespace); only this data source reaches cross-namespace.
data "terraform_remote_state" "zitadel_bootstrap" {
  backend = "kubernetes"

  config = {
    secret_suffix     = "zitadel-bootstrap-identity"
    namespace         = "zitadel"
    in_cluster_config = true
  }
}

locals {
  org_id        = data.terraform_remote_state.zitadel_bootstrap.outputs.org_id
  admin_user_id = data.terraform_remote_state.zitadel_bootstrap.outputs.admin_user_id
  user_user_id  = data.terraform_remote_state.zitadel_bootstrap.outputs.user_user_id
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

# Grants bind the stored bootstrap user IDs directly (no email lookups).
# Admin (super-admin) gets seaweedfs-admin; the normal user gets
# seaweedfs-user. The filer UI stays admin-only via the proxy's
# --allowed-group=seaweedfs-admin (see ui-auth.yaml); the user grant exists so
# a future read-only gate can bind it without touching the project.
resource "zitadel_user_grant" "admin" {
  org_id     = local.org_id
  project_id = zitadel_project.seaweedfs.id
  user_id    = local.admin_user_id
  role_keys  = ["seaweedfs-admin"]
}

resource "zitadel_user_grant" "user" {
  org_id     = local.org_id
  project_id = zitadel_project.seaweedfs.id
  user_id    = local.user_user_id
  role_keys  = ["seaweedfs-user"]
}

# oauth2-proxy cookie secret, generated in-Tofu (32 random bytes, base64 —
# mirrors `openssl rand -base64 32`). Stored ONLY in the outputs Secret, never
# Git, never Proton Pass; regenerated iff the state is recreated.
resource "random_bytes" "cookie_secret" {
  length = 32
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups). Name stays `seaweedfs`. The generated client_id/client_secret are
# computed server-side and flow to the proxy through the
# `seaweedfs-sso-outputs` Secret (writeOutputsToSecret in terraform.yaml) —
# no Proton Pass seeding, never Git. Redirect
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
