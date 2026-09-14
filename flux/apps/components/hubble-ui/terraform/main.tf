# Hubble-ui SSO identity (§11.3), machine-applied by the `hubble-ui-sso`
# Terraform CR (base/terraform.yaml, Tofu Controller) — thin caller of the
# canonical sso-client module (flux/infra/components/zitadel/terraform/
# modules/sso-client, pinned via git source below). Owns ONLY this app's
# slice: the `hubble-ui` Zitadel project + project-scoped admin role
# (`hubble-ui-admin`) + admin grant + the `hubble` OIDC client.
# Project-scoped means hubble-ui-admin NEVER implies org admin.
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

# Admin-only behind oauth2-proxy: no user role, generated in-Tofu cookie
# secret (32 random bytes, base64). Redirect https://<app-host>/* covers the
# proxy callback /oauth2/callback; post-logout https://<app-host>/; app_host
# rides the per-env overlay CR vars. Generated client_id/client_secret land in
# the `hubble-ui-sso-outputs` Secret via the CR's writeOutputsToSecret and are
# consumed natively (k8s-provider ExternalSecret), never Git, never Proton Pass.
module "sso" {
  source = "git::https://github.com/LazyGeniusMan/home-ops.git//flux/infra/components/zitadel/terraform/modules/sso-client?ref=d416199c6acd37d94f9b79f297ce534fea82427d"

  project_name              = "hubble-ui"
  oidc_name                 = "hubble"
  org_id                    = var.org_id
  admin_user_id             = var.admin_user_id
  redirect_uris             = ["https://${var.app_host}/*"]
  post_logout_redirect_uris = ["https://${var.app_host}/"]
  create_user_role          = false
  create_cookie_secret      = true
}
