# Coder SSO identity (§13.6), machine-applied by the `coder-sso` Terraform CR
# (base/terraform.yaml, Tofu Controller) — thin caller of the canonical
# sso-client module
# (flux/infra/components/zitadel/terraform/modules/sso-client). Owns ONLY this
# app's slice: its Zitadel project + project-scoped roles (coder-admin /
# coder-user) + user grants + OIDC client.
# Provider auth: the FirstInstance machine user (`zitadel-bootstrap-sa`,
# IAM_OWNER) via JWT profile — the controller injects var.jwt_profile_json from
# the ESO-synced `coder-terraform-vars` Secret (Kubernetes-provider mirror of
# the chart handoff `zitadel-bootstrap-credentials` in the `zitadel` namespace,
# never Git, never Proton Pass); for manual runs pass
# -var jwt_profile_json="$(cat <key>.json)" instead.
# Org anchor: IDs flow from the FirstInstance handoff (`zitadel-bootstrap-outputs`
# Secret in the `zitadel` namespace: org_id + admin_user_id, operator-created once
# per the zitadel README runbook) via the ESO-synced `coder-terraform-vars`
# Secret (same-namespace `varsFrom` in base/terraform.yaml — a cross-namespace
# `coder-zitadel` SecretStore + the narrow `coder-zitadel-handoff-reader` Role in
# base/zitadel-handoff-rbac.yaml do the mirroring). No literal org_id in git, no
# manual per-env fill, no remote-state read off the retired bootstrap state.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# Redirect https://<app-host>/* covers the callback
# /api/v2/users/oidc/callback; app_host rides the per-env overlay CR vars.
# The generated client_id/client_secret are computed server-side — they flow out
# via the module outputs into the `coder-sso-outputs` Secret (CR
# writeOutputsToSecret), which the `coder-oidc` ExternalSecret consumes
# through the in-cluster `coder-k8s` SecretStore (no Proton Pass seeding).
module "sso" {
  source = "git::https://github.com/LazyGeniusMan/home-ops.git//flux/infra/components/zitadel/terraform/modules/sso-client?ref=d416199c6acd37d94f9b79f297ce534fea82427d"

  project_name              = "coder"
  domain                    = var.domain
  jwt_profile_json          = var.jwt_profile_json
  org_id                    = var.org_id
  admin_user_id             = var.admin_user_id
  redirect_uris             = ["https://${var.app_host}/*"]
  post_logout_redirect_uris = ["https://${var.app_host}/"]
  create_user_role          = true
  user_emails               = var.user_emails
  user_initial_password     = var.user_initial_password
}
