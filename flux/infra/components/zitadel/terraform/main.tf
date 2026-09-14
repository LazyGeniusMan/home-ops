# Reusable Zitadel SSO root — shipped inside the infra/zitadel OCI artifact
# (flux/infra/components/zitadel/terraform/). Consumers reference this root
# via cross-namespace sourceRef + their own vars instead of owning a thin
# root pinned to a git SHA of the sso-client module.
#
# This root owns NO resources directly: provider auth + a single local module
# call (`./modules/sso-client`) that owns the full SSO slice (project +
# project-scoped roles + user grants + OIDC client + optional cookie
# secret). Callers render redirect URIs from their own app_host/ui_host
# vars — this root takes no app_host/ui_host vars.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

module "sso" {
  source = "./modules/sso-client"

  project_name              = var.project_name
  group_name                = var.group_name
  oidc_name                 = var.oidc_name
  domain                    = var.domain
  jwt_profile_json          = var.jwt_profile_json
  org_id                    = var.org_id
  admin_user_id             = var.admin_user_id
  admin_emails              = var.admin_emails
  redirect_uris             = var.redirect_uris
  post_logout_redirect_uris = var.post_logout_redirect_uris
  admin_role_key            = var.admin_role_key
  create_user_role          = var.create_user_role
  user_role_key             = var.user_role_key
  user_emails               = var.user_emails
  user_initial_password     = var.user_initial_password
  create_cookie_secret      = var.create_cookie_secret
}
