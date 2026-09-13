# SeaweedFS filer-UI SSO identity (§12 follow-up), machine-applied by the
# `seaweedfs-sso` Terraform CR (configs/base/terraform.yaml, Tofu Controller)
# — owns ONLY this component's slice: its Zitadel project + project-scoped
# roles + user grants + the `seaweedfs` OIDC client for the oauth2-proxy UI
# gate. The public S3 API route stays DIRECT (SigV4-gated machine endpoint,
# see the component README §12 decision) — no client needed for S3.
# Provider auth: the FirstInstance machine user (`zitadel-bootstrap-sa`,
# IAM_OWNER) via JWT profile — the controller injects var.jwt_profile_json from
# the ESO-synced `seaweedfs-terraform-vars` Secret (Kubernetes-provider mirror of
# the chart handoff `zitadel-bootstrap-credentials` in the `zitadel` namespace,
# never Git, never Proton Pass); for manual runs pass
# -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Org + user anchor: IDs come from the FirstInstance handoff
# (`zitadel-bootstrap-outputs` Secret in the `zitadel` namespace: org_id +
# admin_user_id, operator-created once per the zitadel README runbook) via the
# ESO-synced `seaweedfs-terraform-vars` Secret (same-namespace `varsFrom` in
# configs/base/terraform.yaml — a cross-namespace `seaweedfs-zitadel` SecretStore + the
# narrow `seaweedfs-zitadel-handoff-reader` Role in configs/base/zitadel-handoff-rbac.yaml do the mirroring). No
# org_id literal in git, no email lookups, no remote-state read off the retired
# bootstrap state. Own state keeps the default backend (state Secrets in the
# seaweedfs namespace).
locals {
  org_id        = var.org_id
  admin_user_id = var.admin_user_id
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

# Grant binds the stored handoff admin ID directly (no email lookups).
# Admin (super-admin) gets seaweedfs-admin. The filer UI stays admin-only via
# the proxy's --allowed-group=seaweedfs-admin (see ui-auth.yaml; no user
# grant).
resource "zitadel_user_grant" "admin" {
  org_id     = local.org_id
  project_id = zitadel_project.seaweedfs.id
  user_id    = local.admin_user_id
  role_keys  = ["seaweedfs-admin"]
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
