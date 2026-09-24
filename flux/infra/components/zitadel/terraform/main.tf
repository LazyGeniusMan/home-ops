# Reusable Zitadel SSO root — shipped inside the infra/zitadel OCI artifact
# (flux/infra/components/zitadel/terraform/). Consumers reference this root
# via cross-namespace sourceRef + their own vars.
#
# Flat single layer: provider auth + the full SSO slice (project +
# project-scoped roles + user grants + OIDC client + optional cookie secret)
# directly — no child modules. Callers render redirect URIs from their own
# app_host/ui_host vars — this root takes no app_host/ui_host vars.
#
# Upsert-only: every managed resource below carries
# `lifecycle { prevent_destroy = true }`, so any plan that would delete or
# replace a resource fails instead of destroying it. Together with the
# explicit `destroy: false` + `destroyResourcesOnDeletion: false` on every
# consumer Terraform CR, there is no `tofu destroy` path via Flux; drift
# detection stays on and outputs are unchanged.
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

# SSO slice: one Zitadel project + project-scoped roles + user grants + OIDC
# client (code flow + PKCE, refresh tokens). Consumers reference this root
# directly via cross-namespace sourceRef (see README.md).
#
# Roles/grants are scoped to THIS project, so `<app>-admin` never implies org
# admin. project_role_check requires a grant to authenticate;
# project_role_assertion puts the roles in the token `groups` claim.
locals {
  group_name     = coalesce(var.group_name, var.project_name)
  oidc_name      = coalesce(var.oidc_name, var.project_name)
  admin_role_key = coalesce(var.admin_role_key, "${var.project_name}-admin")
  user_role_key  = coalesce(var.user_role_key, "${var.project_name}-user")
  # Title-case display names mirror the consumer convention ("Coder Admin").
  display_prefix = join(" ", [for w in split("-", var.project_name) : "${upper(substr(w, 0, 1))}${substr(w, 1, -1)}"])
}

# Org anchor: IDs flow from the FirstInstance handoff (`zitadel-bootstrap-outputs`
# Secret in the `zitadel` namespace: org_id + admin_user_id, operator-created once
# per the zitadel README runbook) via the ESO-synced `<app>-terraform-vars`
# Secret (same-namespace `varsFrom` in the consumer's base/terraform.yaml).
# No literal org_id in git, no manual per-env fill, no remote-state reads.
data "zitadel_org" "home_ops" {
  id = var.org_id
}

resource "zitadel_project" "this" {
  org_id                 = data.zitadel_org.home_ops.id
  name                   = var.project_name
  project_role_check     = true
  project_role_assertion = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "zitadel_project_role" "admin" {
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.this.id
  role_key     = local.admin_role_key
  display_name = "${local.display_prefix} Admin"
  group        = local.group_name

  lifecycle {
    prevent_destroy = true
  }
}

resource "zitadel_project_role" "user" {
  count        = var.create_user_role ? 1 : 0
  org_id       = data.zitadel_org.home_ops.id
  project_id   = zitadel_project.this.id
  role_key     = local.user_role_key
  display_name = "${local.display_prefix} User"
  group        = local.group_name

  lifecycle {
    prevent_destroy = true
  }
}

# Admin comes from the FirstInstance handoff var (stored ID — no email lookup
# needed); normal users are owned HERE (per-app decoupling — the bootstrap is
# admin-only). Empty user_emails = admin-only.
resource "zitadel_user_grant" "admin" {
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.this.id
  user_id    = var.admin_user_id
  role_keys  = [local.admin_role_key]

  lifecycle {
    prevent_destroy = true
  }
}

# Extra OIDC admins beyond the bootstrap admin (admin_emails): created as
# human users here + granted the admin project role. Empty = bootstrap admin
# only. Shares user_initial_password (null = invite/reset flow, never git).
resource "zitadel_human_user" "admins" {
  for_each          = toset(var.admin_emails)
  org_id            = data.zitadel_org.home_ops.id
  user_name         = each.value
  first_name        = "Home-Ops"
  last_name         = "Admin"
  email             = each.value
  is_email_verified = true
  initial_password  = var.user_initial_password

  lifecycle {
    prevent_destroy = true
  }
}

resource "zitadel_org_member" "admins" {
  for_each = zitadel_human_user.admins
  org_id   = data.zitadel_org.home_ops.id
  user_id  = each.value.id
  roles    = []

  lifecycle {
    prevent_destroy = true
  }
}

resource "zitadel_user_grant" "extra_admins" {
  for_each   = zitadel_human_user.admins
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.this.id
  user_id    = each.value.id
  role_keys  = [local.admin_role_key]

  lifecycle {
    prevent_destroy = true
  }
}

# Normal users: plain org members (roles=[]) + user-role grant. App-side
# ownership/RBAC distinguishes them from the admin (see the app README).
resource "zitadel_human_user" "users" {
  for_each          = toset(var.user_emails)
  org_id            = data.zitadel_org.home_ops.id
  user_name         = each.value
  first_name        = "Home-Ops"
  last_name         = "User"
  email             = each.value
  is_email_verified = true
  initial_password  = var.user_initial_password

  lifecycle {
    prevent_destroy = true
  }
}

resource "zitadel_org_member" "users" {
  for_each = zitadel_human_user.users
  org_id   = data.zitadel_org.home_ops.id
  user_id  = each.value.id
  roles    = []

  lifecycle {
    prevent_destroy = true
  }
}

resource "zitadel_user_grant" "users" {
  for_each   = zitadel_human_user.users
  org_id     = data.zitadel_org.home_ops.id
  project_id = zitadel_project.this.id
  user_id    = each.value.id
  role_keys  = [local.user_role_key]

  lifecycle {
    prevent_destroy = true
  }
}

# oauth2-proxy cookie secret, generated in-Tofu (32 random bytes, base64 —
# mirrors `openssl rand -base64 32`). Stored ONLY in the outputs Secret, never
# Git, never Proton Pass; regenerated iff the state is recreated.
resource "random_bytes" "cookie_secret" {
  count  = var.create_cookie_secret ? 1 : 0
  length = 32

  lifecycle {
    prevent_destroy = true
  }
}

# Trusted login domain (§11.1 split): the NetBird-exposed login host is
# registered as an instance trusted domain (not for routing — for API
# responses like OIDC discovery + login redirects) so Zitadel serves auth
# flows on it without "Instance not found" (custom-domain.mdx). Requires
# iam.write on the bootstrap machine user. Derives the bare host from
# var.login_base_uri (strips scheme + /ui/v2/login path); count-gated so
# callers that pass no login URI get no extra resource.
resource "zitadel_instance_trusted_domain" "login" {
  count       = var.login_base_uri != null && var.login_base_uri != "" ? 1 : 0
  instance_id = data.zitadel_org.home_ops.id
  domain      = regex("https?://([^/]+)", var.login_base_uri)[0]

  lifecycle {
    prevent_destroy = true
  }
}

# OIDC client (code flow + PKCE, refresh tokens; scopes openid profile email
# groups). The generated client_id/client_secret are computed server-side —
# they flow out via the root outputs into the `<app>-sso-outputs` Secret (CR
# writeOutputsToSecret), consumed by the app's ExternalSecret through the
# in-cluster `<app>-k8s` SecretStore (no Proton Pass seeding).
# login_version.login_v2.base_uri (dynamic: only when var.login_base_uri is
# set, mirroring client_converter.go — clients with a specific URI redirect
# auth requests to the login host; without it they fall back to the instance
# LoginV2 default) points each client at the NetBird-exposed login UI.
resource "zitadel_application_oidc" "this" {
  org_id                      = data.zitadel_org.home_ops.id
  project_id                  = zitadel_project.this.id
  name                        = local.oidc_name
  redirect_uris               = var.redirect_uris
  post_logout_redirect_uris   = var.post_logout_redirect_uris
  response_types              = ["OIDC_RESPONSE_TYPE_CODE"]
  grant_types                 = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN"]
  app_type                    = "OIDC_APP_TYPE_WEB"
  auth_method_type            = "OIDC_AUTH_METHOD_TYPE_BASIC"
  access_token_type           = "OIDC_TOKEN_TYPE_BEARER"
  access_token_role_assertion = true
  id_token_role_assertion     = true
  id_token_userinfo_assertion = true

  dynamic "login_version" {
    for_each = var.login_base_uri != null && var.login_base_uri != "" ? [var.login_base_uri] : []
    content {
      login_v2 {
        base_uri = login_version.value
      }
    }
  }

  lifecycle {
    prevent_destroy = true
  }
}
