# SeaweedFS filer-UI SSO identity (§12 follow-up), machine-applied by the
# `seaweedfs-sso` Terraform CR (configs/base/terraform.yaml, Tofu Controller)
# — thin caller of the canonical sso-client module
# (flux/infra/components/zitadel/terraform/modules/sso-client, git-sourced:
# Tofu Controller sources are per-component OCI artifacts that cannot `path:`
# into flux/infra directly). Owns ONLY this component's slice: its Zitadel
# project + project-scoped roles + user grants + the `seaweedfs` OIDC client
# for the oauth2-proxy UI gate. The public S3 API route stays DIRECT
# (SigV4-gated machine endpoint, see the component README §12 decision) —
# no client needed for S3.
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

# Admin-only filer-UI proxy gate: exact redirect
# https://<ui-host>/oauth2/callback (ui_host rides the per-env overlay CR
# vars); in-Tofu cookie secret; no user role. The generated
# client_id/client_secret flow to the proxy through the
# `seaweedfs-sso-outputs` Secret (writeOutputsToSecret in terraform.yaml) —
# no Proton Pass seeding, never Git.
module "sso" {
  source = "git::https://github.com/LazyGeniusMan/home-ops.git//flux/infra/components/zitadel/terraform/modules/sso-client?ref=d416199c6acd37d94f9b79f297ce534fea82427d"

  project_name              = "seaweedfs"
  org_id                    = var.org_id
  admin_user_id             = var.admin_user_id
  redirect_uris             = ["https://${var.ui_host}/oauth2/callback"]
  post_logout_redirect_uris = ["https://${var.ui_host}/"]
  create_user_role          = false
  create_cookie_secret      = true
}
