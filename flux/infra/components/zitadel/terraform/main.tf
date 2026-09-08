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

resource "zitadel_human_user" "user" {
  org_id            = zitadel_org.home_ops.id
  user_name         = var.user_email
  first_name        = "Home-Ops"
  last_name         = "User"
  email             = var.user_email
  is_email_verified = true
  initial_password  = var.user_initial_password
}

# Groups: the pinned provider (zitadel ~> 3.3) ships no zitadel_user_group
# resources, so group membership is expressed with org/project membership:
# admin ≡ ORG_OWNER + project admin grant; users ≡ plain org member + project
# user grant. The `groups` claim on every per-app OIDC client still carries
# admin/users via that app's own project role assertion + grants.
resource "zitadel_org_member" "admin_member" {
  org_id  = zitadel_org.home_ops.id
  user_id = zitadel_human_user.admin.id
  roles   = ["ORG_OWNER"]
}

resource "zitadel_org_member" "user_member" {
  org_id  = zitadel_org.home_ops.id
  user_id = zitadel_human_user.user.id
  roles   = []
}

# Roles/grants: home-ops project with admin/user roles; user grants bind each
# user to their role (drives id_token role assertion).
resource "zitadel_project" "home_ops" {
  org_id               = zitadel_org.home_ops.id
  name                 = "home-ops"
  project_role_check   = true
  project_role_assertion = true
}

resource "zitadel_project_role" "admin" {
  org_id     = zitadel_org.home_ops.id
  project_id = zitadel_project.home_ops.id
  role_key   = "admin"
  display_name = "Admin"
  group      = "home-ops"
}

resource "zitadel_project_role" "user" {
  org_id     = zitadel_org.home_ops.id
  project_id = zitadel_project.home_ops.id
  role_key   = "user"
  display_name = "User"
  group      = "home-ops"
}

resource "zitadel_user_grant" "admin" {
  org_id     = zitadel_org.home_ops.id
  project_id = zitadel_project.home_ops.id
  user_id    = zitadel_human_user.admin.id
  role_keys  = ["admin"]
}

resource "zitadel_user_grant" "user" {
  org_id     = zitadel_org.home_ops.id
  project_id = zitadel_project.home_ops.id
  user_id    = zitadel_human_user.user.id
  role_keys  = ["user"]
}

# OIDC clients: none here — each app owns its own `zitadel_project` +
# `zitadel_application_oidc` client in its per-app `terraform/` slice (coder,
# clickstack, hubble-ui, flux-operator-ui, headlamp, seaweedfs). This
# bootstrap slice keeps only org, users, membership, and the legacy
# `home-ops` project + roles + grants (see the README contract table for the
# per-app ownership map). Redirects ride each app's per-env `app_host` /
# `ui_host` CR vars, so passing -var domain=zitadel.homelab-dev.yansyah.my.id
# (+ admin/user emails) switches every redirect to dev with no other edits
# (§14).
