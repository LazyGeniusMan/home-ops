# Clickstack SSO identity (§11.2), machine-applied by the `clickstack-sso`
# Terraform CR (base/terraform.yaml, Tofu Controller) — owns ONLY this app's
# slice: its Zitadel project + project-scoped roles + user grants + OIDC
# client.# Provider auth: the FirstInstance machine user (`zitadel-bootstrap-sa`,
# IAM_OWNER) via JWT profile — the controller injects var.jwt_profile_json from
# the ESO-synced `clickstack-terraform-vars` Secret (Kubernetes-provider mirror of
# the chart handoff `zitadel-bootstrap-credentials` in the `zitadel` namespace,
# never Git, never Proton Pass); for manual runs pass
# -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Org anchor: IDs flow from the FirstInstance handoff (`zitadel-bootstrap-outputs`
# Secret in the `zitadel` namespace: org_id + admin_user_id, operator-created once
# per the zitadel README runbook) via the ESO-synced `clickstack-terraform-vars`
# Secret (same-namespace `varsFrom` in base/terraform.yaml — a cross-namespace
# `clickstack-zitadel` SecretStore + the narrow `clickstack-zitadel-handoff-reader` Role in
# base/zitadel-handoff-rbac.yaml do the mirroring). No literal org_id in git, no
# manual per-env fill, no remote-state read off the retired bootstrap state.
data "zitadel_org" "home_ops" {
  id = var.org_id
}

# App project: the roles/grants below are scoped HERE, so `clickstack-admin`
# never implies org admin (project-scoped role only). project_role_check
# requires a grant to authenticate; project_role_assertion puts the roles in
# the token `groups` claim.
resource "zitadel_project" "clickstack" {
  org_id                 = data.zitadel_org.home_ops.id
  name                   = "clickstack"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.clickstack.id
  role_key     = "clickstack-admin"
  display_name = "Clickstack Admin"
  group        = "clickstack"
}

# Admin user comes from the FirstInstance handoff var (stored ID —
# no email lookup needed). Admin (super-admin, ORG_OWNER) gets clickstack-admin.
# Only clickstack-admin may sign in — the per-app oauth2-proxy sidecar gates
# on `--allowed-group=clickstack-admin` (admin-only UI, no user grant).
locals {
  admin_user_id = var.admin_user_id
}

resource "zitadel_user_grant" "admin" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.clickstack.id
  user_id    = local.admin_user_id
  role_keys  = ["clickstack-admin"]
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups). Relocated verbatim from the `clickstack` entry of the central
# zitadel module's client for_each (name + wildcard redirect
# https://<app-host>/* covers the proxy callback /oauth2/callback — same as
# the shared client `https://*/oauth2/callback` covered). app_host rides the
# per-env overlay CR vars. The generated client_id/client_secret are computed
# server-side — they flow out via the module outputs into the
# `clickstack-sso-outputs` Secret (CR writeOutputsToSecret), which the
# `oauth2-proxy` ExternalSecret consumes through the in-cluster
# `clickstack-k8s` SecretStore (no Proton Pass seeding for OIDC creds).
resource "zitadel_application_oidc" "clickstack" {
  org_id                      = data.zitadel_org.home_ops.id
  project_id                  = zitadel_project.clickstack.id
  name                        = "clickstack"
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
