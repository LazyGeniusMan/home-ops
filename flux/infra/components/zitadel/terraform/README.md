# Reusable Zitadel SSO root

Single reusable Terraform root for every app SSO slice, shipped inside the
`infra/zitadel` OCI artifact. Consumers reference this root via
cross-namespace `sourceRef` + their own vars.

## Layout (flat — no child modules)

- `main.tf` — provider `zitadel` block + the full SSO slice inline: org
  lookup, project, admin project role + bootstrap-admin grant, optional user
  role, human users + org members + grants (normal users plus extra admins
  from `admin_emails`), OIDC app with coder secure defaults, optional
  `random_bytes` cookie secret. Zero `module` blocks.
- `variables.tf` — full contract (project, credentials, roles, users, URIs,
  cookie secret). No `app_host`/`ui_host` vars: callers pass fully-rendered
  `redirect_uris` / `post_logout_redirect_uris`.
- `outputs.tf` — `client_id`, `client_secret` (sensitive), `project_id`,
  `cookie_secret` (sensitive, `""` when disabled), `admin_role_key`,
  `user_role_key`. Names match what consumer `writeOutputsToSecret`
  expects (`client_id`/`client_secret`[`/cookie_secret`]).
- `versions.tf` — `required_version >= 1.11`, `zitadel/zitadel ~> 3.3`,
  `hashicorp/random ~> 3.7`.

## Consumer usage (cross-namespace sourceRef)

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: <app>-sso
spec:
  sourceRef:
    kind: OCIRepository
    name: infra-zitadel # cross-namespace reference to the infra artifact
  path: ./terraform # this reusable root
  vars:
    - name: domain
      value: zitadel.home-ops.yansyah.my.id
    - name: project_name
      value: <app>
    - name: redirect_uris
      value:
        - https://<app-host>/oauth2/callback
    - name: post_logout_redirect_uris
      value:
        - https://<app-host>/
    - name: create_cookie_secret
      value: "true" # oauth2-proxy pattern; omit for direct-OIDC apps
  varsFrom:
    - kind: Secret
      name: <app>-terraform-vars # jwt_profile_json, org_id, admin_user_id
  writeOutputsToSecret:
    name: <app>-sso-outputs
    outputs:
      - client_id
      - client_secret
      - cookie_secret # only when create_cookie_secret is true
```

User-capable apps add `create_user_role: "true"` + `user_emails` (+ optional
`user_initial_password` via `varsFrom`, never git). Extra OIDC admins beyond
the bootstrap admin go in `admin_emails` (created + granted the admin role).
Provider auth (`jwt_profile_json`) and handoff IDs (`org_id`,
`admin_user_id`) flow from the FirstInstance handoff
(`zitadel-bootstrap-credentials` / `zitadel-bootstrap-outputs` Secrets in
the `zitadel` namespace) via the ESO-synced `<app>-terraform-vars` Secret —
never commit secrets, never hardcode IDs.

## Variables

| Name | Type | Required | Default | Description |
| ---- | ---- | -------- | ------- | ----------- |
| `project_name` | `string` | yes | — | Zitadel project name; fallback for `group_name`, `oidc_name`, role keys |
| `group_name` | `string` | no | `project_name` | Group claim value on the project roles (oauth2-proxy `--allowed-group` / app group gates) |
| `oidc_name` | `string` | no | `project_name` | Display name of the OIDC application |
| `domain` | `string` | no | `zitadel.home-ops.yansyah.my.id` | Zitadel external domain (issuer host, no scheme) |
| `jwt_profile_json` | `string` (sensitive) | no | `null` | JWT profile key JSON for the FirstInstance IAM_OWNER machine user (controller injects via `varsFrom`; manual runs pass `-var`, never commit) |
| `org_id` | `string` | no | `null` | Home-ops org ID from the FirstInstance handoff (`zitadel-bootstrap-outputs` Secret via ESO mirror) |
| `admin_user_id` | `string` (sensitive) | no | `null` | Bootstrap admin user ID from the FirstInstance handoff — always granted the admin project role |
| `admin_emails` | `list(string)` | no | `[]` | Extra OIDC admins beyond the bootstrap admin (created as human users + granted the admin role) |
| `redirect_uris` | `list(string)` | yes | — | Fully-rendered OIDC redirect URIs — callers render hosts; the root takes no `app_host`/`ui_host` vars |
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
# consumer Terraform CR vars (path: ./terraform):
project_name              = "seaweedfs"
redirect_uris             = ["https://<ui-host>/oauth2/callback"]
post_logout_redirect_uris = ["https://<ui-host>/"]
create_cookie_secret      = true
```

User-capable app (coder / headlamp pattern — wildcard redirect, user role +
user creation):

```hcl
# consumer Terraform CR vars (path: ./terraform):
project_name              = "coder"
redirect_uris             = ["https://<app-host>/*"]
post_logout_redirect_uris = ["https://<app-host>/"]
create_user_role          = true
user_emails               = var.user_emails
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

## Secure defaults

- `project_role_check = true` + `project_role_assertion = true`: a grant is
  required to authenticate and roles land in the token `groups` claim.
- Roles/grants are project-scoped — `<app>-admin` never implies org admin.
- OIDC: `CODE` response, authorization-code + refresh grants, `WEB` app
  type, `BASIC` auth method, `BEARER` token type, all role/userinfo
  assertions on.
- No secrets in git: JWT profile, org/admin IDs, and generated
  client/cookie secrets flow through ESO mirrors and the CR outputs Secret.
