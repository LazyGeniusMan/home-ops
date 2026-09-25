# Reusable Zitadel SSO root

Single reusable Terraform root for every app SSO slice, shipped inside the
`infra/zitadel` OCI artifact. Consumers reference this root via
cross-namespace `sourceRef` + their own vars.

## Layout (flat — no child modules)

- `main.tf` — provider `zitadel` block + the full SSO slice inline: org
  lookup, project, admin role + bootstrap-admin grant, optional user role,
  users + members + grants, OIDC app, optional `random_bytes` cookie secret.
  Zero `module` blocks.
- `variables.tf` — full contract (project, credentials, roles, users, URIs,
  cookie secret). No `app_host`/`ui_host` vars: callers pass fully-rendered
  `redirect_uris` / `post_logout_redirect_uris`. See `variables.tf` for the
  variable table.
- `outputs.tf` — `client_id`, `client_secret` (sensitive), `project_id`,
  `cookie_secret` (sensitive, `""` when disabled), `admin_role_key`,
  `user_role_key`. Names match what consumer `writeOutputsToSecret` expects
  (`client_id`/`client_secret`[`/cookie_secret`]). See `outputs.tf` for the
  output table.
- `versions.tf` — `required_version >= 1.11`, `zitadel/zitadel ~> 3.3`,
  `hashicorp/random ~> 3.7`.

## Consumer usage (cross-namespace sourceRef)

Consumer Terraform CR: `sourceRef` the `infra` OCIRepository in namespace
`zitadel` at `path: ./terraform`, plain `vars` for `domain` / `project_name` /
`redirect_uris` / `post_logout_redirect_uris` (+ `create_cookie_secret: "true"`
for the oauth2-proxy pattern), `varsFrom` the `<app>-terraform-vars` Secret
(`jwt_profile_json`, `org_id`, `admin_user_id` from the FirstInstance handoff
via ESO), `writeOutputsToSecret` to `<app>-sso-outputs` (`client_id` /
`client_secret` [/ `cookie_secret`]). User-capable apps add
`create_user_role: "true"` + `user_emails`; extra admins go in `admin_emails`.
Two shapes: admin-only behind oauth2-proxy (exact callback URI, generated
cookie secret — hubble-ui / seaweedfs pattern) and user-capable app (wildcard
redirect, user role + user creation — coder / headlamp pattern).

## Secure defaults

- Upsert-only: every managed resource carries
  `lifecycle { prevent_destroy = true }`, and every consumer Terraform CR
  sets `destroy: false` + `destroyResourcesOnDeletion: false`. No `tofu
  destroy` path via Flux; drift detection stays on.
- `project_role_check = true` + `project_role_assertion = true`: a grant is
  required to authenticate and roles land in the token `groups` claim.
- Roles/grants are project-scoped — `<app>-admin` never implies org admin.
- OIDC: `CODE` response, authorization-code + refresh grants, `WEB` app
  type, `BASIC` auth method, `BEARER` token type, all role/userinfo
  assertions on.
- No secrets in git: JWT profile, org/admin IDs, and generated
  client/cookie secrets flow through ESO mirrors and the CR outputs Secret.
