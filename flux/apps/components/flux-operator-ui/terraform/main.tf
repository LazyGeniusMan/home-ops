# Flux Operator UI SSO identity (§11.4), machine-applied by the
# `flux-operator-ui-sso` Terraform CR (base/terraform.yaml, Tofu Controller)
# — thin caller of the canonical sso-client module: owns ONLY this app's
# slice (its Zitadel project + project-scoped admin role + admin grant +
# OIDC client). Admin-only behind oauth2-proxy (`--allowed-group`
# flux-operator-ui-admin, no user grant); the cookie secret stays in Vault
# (no in-Terraform cookie).
# Provider auth: the FirstInstance machine user (`zitadel-bootstrap-sa`,
# IAM_OWNER) via JWT profile — the controller injects var.jwt_profile_json from
# the ESO-synced `flux-operator-ui-terraform-vars` Secret (Kubernetes-provider mirror of
# the chart handoff `zitadel-bootstrap-credentials` in the `zitadel` namespace,
# never Git, never Proton Pass); for manual runs pass
# -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

module "sso" {
  source = "git::https://github.com/LazyGeniusMan/home-ops.git//flux/infra/components/zitadel/terraform/modules/sso-client?ref=d416199c6acd37d94f9b79f297ce534fea82427d"

  project_name              = "flux-operator-ui"
  org_id                    = var.org_id
  admin_user_id             = var.admin_user_id
  redirect_uris             = ["https://${var.app_host}/*"]
  post_logout_redirect_uris = ["https://${var.app_host}/"]
  create_user_role          = false
  create_cookie_secret      = false
}
