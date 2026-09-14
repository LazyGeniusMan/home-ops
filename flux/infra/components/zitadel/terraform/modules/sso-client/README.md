# sso-client — canonical Zitadel SSO slice

One Zitadel project + project-scoped roles + user grants + OIDC client
(code flow + PKCE, refresh tokens), previously copy-pasted across the
per-consumer `terraform/` roots (coder, headlamp, clickstack, hubble-ui,
flux-operator-ui, seaweedfs).

## Why git-sourced (not `path:`)

Tofu Controller sources are per-component OCI artifacts (`apps/<component>`,
`infra/<component>`), so a Terraform CR cannot `path:` into `flux/infra`
directly. Consumers therefore keep a thin per-app root (provider block +
`module "sso"` + vars) and pull this canonical module via a git source.
No CR contract change is needed — the CR still points at the consumer's own
`terraform/` dir.

## Thin-root usage

```hcl
provider "zitadel" {
  domain           = var.domain
  jwt_profile_json = var.jwt_profile_json
}

module "sso" {
  source = "git::https://github.com/LazyGeniusMan/home-ops.git//flux/infra/components/zitadel/terraform/modules/sso-client?ref=<sha>"

  project_name              = "coder"
  org_id                    = var.org_id
  admin_user_id             = var.admin_user_id
  redirect_uris             = ["https://${var.app_host}/*"]
  post_logout_redirect_uris = ["https://${var.app_host}/"]
}
```

> [!WARNING]
> Pin `?ref=` to a full commit SHA and bump it deliberately. The module floats
> independently of the dev/stable OCI tags — a branch ref would silently move
> every consumer on the next Tofu Controller run.

## Variables

| Name | Type | Required | Default | Description |
| ---- | ---- | -------- | ------- | ----------- |
| `project_name` | `string` | yes | — | Zitadel project name; fallback for `group_name`, `oidc_name`, role keys |
| `group_name` | `string` | no | `project_name` | Group claim value on the project roles (oauth2-proxy `--allowed-group` / app group gates) |
| `oidc_name` | `string` | no | `project_name` | Display name of the OIDC application |
| `domain` | `string` | no | `zitadel.home-ops.yansyah.my.id` | Zitadel external domain (issuer host, no scheme) |
| `jwt_profile_json` | `string` (sensitive) | no | `null` | JWT profile key JSON for the FirstInstance IAM_OWNER machine user (controller injects via `varsFrom`; manual runs pass `-var`, never commit) |
| `org_id` | `string` | no | `null` | Home-ops org ID from the FirstInstance handoff (`zitadel-bootstrap-outputs` Secret via ESO mirror) |
| `admin_user_id` | `string` (sensitive) | no | `null` | Bootstrap admin user ID from the FirstInstance handoff |
| `redirect_uris` | `list(string)` | yes | — | Fully-rendered OIDC redirect URIs — callers render hosts; the module takes no `app_host`/`ui_host` vars |
| `post_logout_redirect_uris` | `list(string)` | yes | — | Fully-rendered post-logout redirect URIs |
| `admin_role_key` | `string` | no | `${project_name}-admin` | Project role key granted to the bootstrap admin |
| `create_user_role` | `bool` | no | `false` | Create the non-admin project role (coder/headlamp pattern) |
| `user_role_key` | `string` | no | `${project_name}-user` | Project role key granted to normal users |
| `user_emails` | `list(string)` | no | `[]` | Normal users to create + grant the user role (empty = admin-only) |
| `user_initial_password` | `string` (sensitive) | no | `null` | Initial password for normal users (rotate after first login) |
| `create_cookie_secret` | `bool` | no | `false` | Generate a 32-byte oauth2-proxy cookie secret (base64 output) |

`group_name`, `oidc_name`, `admin_role_key`, and `user_role_key` default to
`null` (Terraform forbids referencing `project_name` in a variable default);
`main.tf` resolves them with `coalesce` to the values shown above.

## Examples

Admin-only behind oauth2-proxy (hubble-ui / seaweedfs pattern — exact
callback URI, generated cookie secret):

```hcl
module "sso" {
  source = "git::https://github.com/LazyGeniusMan/home-ops.git//flux/infra/components/zitadel/terraform/modules/sso-client?ref=<sha>"

  project_name              = "seaweedfs"
  org_id                    = var.org_id
  admin_user_id             = var.admin_user_id
  redirect_uris             = ["https://${var.ui_host}/oauth2/callback"]
  post_logout_redirect_uris = ["https://${var.ui_host}/"]
  create_cookie_secret      = true
}
```

User-capable app (coder / headlamp pattern — wildcard redirect, user role +
user creation):

```hcl
module "sso" {
  source = "git::https://github.com/LazyGeniusMan/home-ops.git//flux/infra/components/zitadel/terraform/modules/sso-client?ref=<sha>"

  project_name              = "coder"
  org_id                    = var.org_id
  admin_user_id             = var.admin_user_id
  redirect_uris             = ["https://${var.app_host}/*"]
  post_logout_redirect_uris = ["https://${var.app_host}/"]
  create_user_role          = true
  user_emails               = var.user_emails
  user_initial_password     = var.user_initial_password
}
```

## Outputs

| Name | Sensitive | Description |
| ---- | --------- | ----------- |
| `client_id` | yes | Generated OIDC `client_id` (into `<app>-sso-outputs` via `writeOutputsToSecret`) |
| `client_secret` | yes | Generated OIDC `client_secret` (same Secret, never Git) |
| `project_id` | no | ID of the Zitadel project owned by this slice |
| `cookie_secret` | yes | Generated cookie secret, base64 — `""` when `create_cookie_secret` is `false` |
| `admin_role_key` | no | Effective admin role key |
| `user_role_key` | no | Effective user role key |

## Secure defaults (preserved from the coder reference)

- `project_role_check = true` + `project_role_assertion = true`: a grant is
  required to authenticate and roles land in the token `groups` claim.
- Roles/grants are project-scoped — `<app>-admin` never implies org admin.
- OIDC: `CODE` response, authorization-code + refresh grants, `WEB` app type,
  `BASIC` auth method, `BEARER` token type, all role/userinfo assertions on.
- No secrets in git: JWT profile, org/admin IDs, and generated
  client/cookie secrets flow through ESO mirrors and the CR outputs Secret.
