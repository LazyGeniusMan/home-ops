# Hubble-ui SSO identity (§11.3), machine-applied by the `hubble-ui-sso`
# Terraform CR (base/terraform.yaml, Tofu Controller) — owns ONLY this app's
# slice: its Zitadel project + project-scoped roles + user grants + OIDC
# client.
# Provider auth: the FirstInstance machine user (`zitadel-bootstrap-sa`,
# IAM_OWNER) via JWT profile — the controller injects var.jwt_profile_json from
# the ESO-synced `hubble-ui-terraform-vars` Secret (Kubernetes-provider mirror of
# the chart handoff `zitadel-bootstrap-credentials` in the `zitadel` namespace,
# never Git, never Proton Pass); for manual runs pass
# -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Org anchor: IDs flow from the FirstInstance handoff (`zitadel-bootstrap-outputs`
# Secret in the `zitadel` namespace: org_id + admin_user_id, operator-created once
# per the zitadel README runbook) via the ESO-synced `hubble-ui-terraform-vars`
# Secret (same-namespace `varsFrom` in base/terraform.yaml — a cross-namespace
# `hubble-ui-zitadel` SecretStore + the narrow `hubble-ui-zitadel-handoff-reader` Role in
# base/zitadel-handoff-rbac.yaml do the mirroring). No literal org_id in git, no
# manual per-env fill, no remote-state read off the retired bootstrap state.
data "zitadel_org" "home_ops" {
  id = var.org_id
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

# Admin user comes from the FirstInstance handoff var (stored ID —
# no email lookup needed). Admin (super-admin, ORG_OWNER) gets hubble-ui-admin.
# The proxy gate stays admin-only (--allowed-group=hubble-ui-admin), so only
# the admin grant gates access (admin-only UI, no user grant).
locals {
  admin_user_id = var.admin_user_id
}

resource "zitadel_user_grant" "admin" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.hubble_ui.id
  user_id    = local.admin_user_id
  role_keys  = ["hubble-ui-admin"]
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
