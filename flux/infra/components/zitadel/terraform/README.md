# Reusable Zitadel SSO root

Single reusable Terraform root for every app SSO slice, shipped inside the
`infra/zitadel` OCI artifact. Consumers reference this root via
cross-namespace `sourceRef` + their own vars instead of owning a thin root
pinned to a git SHA of the `sso-client` module.

## Layout

- `main.tf` — provider `zitadel` block + `module "sso"` with local source
  `./modules/sso-client`. Zero inline `resource`/`data` blocks: all
  resources live in the child module.
- `variables.tf` — full passthrough of the child module vars (project,
  credentials, roles, users, URIs, cookie secret). No `app_host`/`ui_host`
  vars: callers pass fully-rendered `redirect_uris` /
  `post_logout_redirect_uris`.
- `outputs.tf` — `client_id`, `client_secret` (sensitive), `project_id`,
  `cookie_secret` (sensitive, `""` when disabled), `admin_role_key`,
  `user_role_key`. Names match what consumer `writeOutputsToSecret`
  expects (`client_id`/`client_secret`[`/cookie_secret`]).
- `versions.tf` — `required_version >= 1.11`, `zitadel/zitadel ~> 3.3`,
  `hashicorp/random ~> 3.7`.
- `modules/sso-client/` — canonical SSO slice (project + project-scoped
  roles + user grants + OIDC client + optional cookie secret). See its
  README for the variable matrix.

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

## Secure defaults (from the sso-client module)

- `project_role_check = true` + `project_role_assertion = true`: a grant is
  required to authenticate and roles land in the token `groups` claim.
- Roles/grants are project-scoped — `<app>-admin` never implies org admin.
- OIDC: `CODE` response, authorization-code + refresh grants, `WEB` app
  type, `BASIC` auth method, `BEARER` token type, all role/userinfo
  assertions on.
