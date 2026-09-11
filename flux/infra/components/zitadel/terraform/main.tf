# Identity-as-code for §11.1, machine-applied by the
# `zitadel-bootstrap-identity` Terraform CR (configs/base, Tofu Controller) —
# the single source of truth for the OIDC contract table in the component
# README. Provider auth: a service user with IAM_OWNER (FirstInstance machine
# user) via JWT profile — the controller injects var.jwt_profile_json from the
# ESO-synced `zitadel-terraform-vars` Secret (Proton Pass, never Git); for
# manual runs pass -var jwt_profile_json="$(cat <key>.json)" instead.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

resource "zitadel_org" "home_ops" {
  name = "home-ops"
}

resource "zitadel_human_user" "admin" {
  org_id            = zitadel_org.home_ops.id
  user_name         = var.admin_email
  first_name        = "Home-Ops"
  last_name         = "Admin"
  email             = var.admin_email
  is_email_verified = true
  initial_password  = var.admin_initial_password
}

# Groups: the pinned provider (zitadel ~> 3.3) ships no zitadel_user_group
# resources, so group membership is expressed with org/project membership:
# admin ≡ ORG_OWNER + project admin grant. Non-admin users are owned per
# consumer app, which asserts its own project roles/grants into the `groups`
# claim on its OIDC client.
resource "zitadel_org_member" "admin_member" {
  org_id  = zitadel_org.home_ops.id
  user_id = zitadel_human_user.admin.id
  roles   = ["ORG_OWNER"]
}

# Roles/grants: home-ops project with the admin role; the admin grant binds
# the admin user to it (drives id_token role assertion).
resource "zitadel_project" "home_ops" {
  org_id                 = zitadel_org.home_ops.id
  name                   = "home-ops"
  project_role_check     = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id       = zitadel_org.home_ops.id
  project_id   = zitadel_project.home_ops.id
  role_key     = "admin"
  display_name = "Admin"
  group        = "home-ops"
}

resource "zitadel_user_grant" "admin" {
  org_id     = zitadel_org.home_ops.id
  project_id = zitadel_project.home_ops.id
  user_id    = zitadel_human_user.admin.id
  role_keys  = ["admin"]
}

# OIDC clients: none here — each app owns its own `zitadel_project` +
# `zitadel_application_oidc` client in its per-app `terraform/` slice (coder,
# clickstack, hubble-ui, flux-operator-ui, headlamp, seaweedfs). This
# bootstrap slice keeps only org, admin user, membership, and the legacy
# `home-ops` project + admin role + grant (see the README contract table for
# the per-app ownership map). Redirects ride each app's per-env `app_host` /
# `ui_host` CR vars, so passing -var domain=zitadel.homelab-dev.yansyah.my.id
# (+ admin email) switches every redirect to dev with no other edits (§14).
