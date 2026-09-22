# matrix tenant — vault seed checklist

One-time `pass insert` per env BEFORE first install. Git holds
`remoteRef` keys only, never values. The dev/prd overlays patch every
`remoteRef.key` below from the `__PROTON_PASS_BASE__` placeholder to the
per-env vault path; the Deployment/StatefulSet only starts once ESO syncs
the Secrets (pods pend + Flux retries until then).

Vault naming: `pass://acme-<env>-bdo1-talos-apps-01/<path>` where
`<env>` is `dev` or `prd`. The `pass insert` shape drops the `pass://`
scheme (same convention as `matrix-rooms/terraform/README.md`):

```shell
pass insert 'acme-<env>-bdo1-talos-apps-01/<path>'
```

Field count per env: **13** Proton Pass fields (1 cert-manager + 2
apprise/matrix-rooms + 8 mautrix-discord + 2 element-web). Everything
else in this tenant is minted in-cluster (COSI, Terraform outputs, CNPG)
and needs NO seeding — see "Not vault-seeded" below.

## Per-env seed table

Same 13 rows for `dev` (`acme-dev-bdo1-talos-apps-01`) and `prd`
(`acme-prd-bdo1-talos-apps-01`); only the values differ per env (hosts,
tokens). Env-specific value notes are in the last column.

| # | Vault field (`<prefix>/<path>`) | ExternalSecret → Secret (key) | Consumed by | Value notes |
|---|---|---|---|---|
| 1 | `cert-manager/cloudflare-api-token` | `cloudflare-api-token` → `cloudflare-api-token` (`api-token`) | cert-manager DNS-01 solver for the in-namespace wildcard `Certificate` (`tuwunel-secrets.yaml`; mirrors `cert-manager/configs/base/cluster-issuer.yaml` with the same remoteRef) | Same Cloudflare API token in both envs (zone-scoped) |
| 2 | `matrix-rooms/notifier-bot-token` | `apprise-stateless-urls` → `apprise-stateless-urls` (`stateless-urls`) | `apprise-go-api` Deployment (`APPRISE_STATELESS_URLS` secretKeyRef, optional) — composed into three `matrixs://<token>@<host>/%23<room>?tag=…` fallback legs (`apprise-go-api-secrets.yaml`) | Per-env bot token (`@apprise-dev` vs `@apprise`); never shared across envs |
| 3 | `matrix-rooms/homeserver-host` | (same ES/Secret as #2) | (same — the `<host>` half of the composed URLs) | Bare host, NO scheme: dev `tuwunel.matrix.homelab-dev.yansyah.my.id`, prd `tuwunel.matrix.home-ops.yansyah.my.id` |
| 4 | `mautrix-discord/bot-token` | `mautrix-discord` → `mautrix-discord` (`bot-token`) | Bridge entrypoint: `login-token bot <token>` session auth at runtime, never baked into config | Discord bot token from the Discord developer portal |
| 5 | `mautrix-discord/as-token` | (same ES/Secret, `as-token`) | Bridge registration (`registration.yaml` via `AS_TOKEN` env) + tuwunel appservice `as_token` side | Random ≥64ch; seeded ONCE, then stable forever (PVC persists `registration.yaml`) |
| 6 | `mautrix-discord/hs-token` | (same ES/Secret, `hs-token`) | Bridge registration (`HS_TOKEN` env) + tuwunel appservice `hs_token` side | Same stability contract as `as-token` |
| 7 | `mautrix-discord/avatar-proxy-key` | (same ES/Secret, `avatar-proxy-key`) | Bridge `/mautrix-discord/avatar` relay HMAC (`AVATAR_PROXY_KEY` env) | Random ≥32ch HMAC key |
| 8 | `mautrix-discord/direct-media-server-key` | (same ES/Secret, `direct-media-server-key`) | Bridge federation media signing (`DIRECT_MEDIA_SERVER_KEY` env; synapse `.signing.key` format) | Generate per synapse signing-key format |
| 9 | `mautrix-discord/provisioning-shared-secret` | (same ES/Secret, `provisioning-shared-secret`) | Bridge provisioning API auth (`PROVISIONING_SHARED_SECRET` env) | Random ≥32ch |
| 10 | `mautrix-discord/double-puppet-shared-secret` | (same ES/Secret, `double-puppet-shared-secret`) | Legacy `login_shared_secret_map` double-puppet value (`DOUBLE_PUPPET_SHARED_SECRET` env) | Random ≥32ch |
| 11 | `mautrix-discord/db-password` | `mautrix-discord-db-credentials` → `mautrix-discord-db-credentials` (`connection-url`, templated `postgres://discord:<pw>@mautrix-discord-db-rw.matrix.svc:5432/discord?sslmode=require`) AND `mautrix-discord-db-app-secret` → `mautrix-discord-db-app-secret` (basic-auth `discord`/`<pw>`; username MUST equal `spec.bootstrap.initdb.owner`) | Bridge `DATABASE_URL` env + CNPG `mautrix-discord-db` Cluster initdb owner password | Random ≥32ch; single field feeds BOTH Secrets |
| 12 | `element-web/netbird-pat` | `element-proxy-vars` → `element-proxy-vars` (`netbird_token`) | Terraform `element-proxy` CR via `varsFrom` (NetBird custom-domain + reverse-proxy service registration) | Per-env NetBird PAT |
| 13 | `element-web/cloudflare-api-token` | (same ES/Secret, `cloudflare_api_token`) | (same CR — Cloudflare side of the NetBird registration) | Distinct vault field from #1 (separate consumer), same upstream token value is fine |

Seed commands (dev shown; repeat with `acme-prd-bdo1-talos-apps-01` for prd):

```shell
pass insert 'acme-dev-bdo1-talos-apps-01/cert-manager/cloudflare-api-token'
pass insert 'acme-dev-bdo1-talos-apps-01/matrix-rooms/notifier-bot-token'
pass insert 'acme-dev-bdo1-talos-apps-01/matrix-rooms/homeserver-host'
pass insert 'acme-dev-bdo1-talos-apps-01/mautrix-discord/bot-token'
pass insert 'acme-dev-bdo1-talos-apps-01/mautrix-discord/as-token'
pass insert 'acme-dev-bdo1-talos-apps-01/mautrix-discord/hs-token'
pass insert 'acme-dev-bdo1-talos-apps-01/mautrix-discord/avatar-proxy-key'
pass insert 'acme-dev-bdo1-talos-apps-01/mautrix-discord/direct-media-server-key'
pass insert 'acme-dev-bdo1-talos-apps-01/mautrix-discord/provisioning-shared-secret'
pass insert 'acme-dev-bdo1-talos-apps-01/mautrix-discord/double-puppet-shared-secret'
pass insert 'acme-dev-bdo1-talos-apps-01/mautrix-discord/db-password'
pass insert 'acme-dev-bdo1-talos-apps-01/element-web/netbird-pat'
pass insert 'acme-dev-bdo1-talos-apps-01/element-web/cloudflare-api-token'
```

## Not vault-seeded (in-cluster minted — DO NOT `pass insert`)

- **Tuwunel SSO `client_id`/`client_secret`**: NO `pass://` seeding.
  The companion `tuwunel-sso` Terraform CR (Zitadel project + `tuwunel`
  OIDC client — ships in the follow-up tofu task, not in this tenant)
  writes the `tuwunel-sso-outputs` Secret; ExternalSecret
  `tuwunel-sso-client` reads it through the in-cluster `tuwunel-k8s`
  SecretStore into `tuwunel-sso-client` (`client-id`, `client-secret`),
  consumed via `secretKeyRef` + `CLIENT_SECRET_FILE` mount.
- **Tuwunel S3 media keys** (`tuwunel-s3` Secret): COSI-minted. The
  `tuwunel-media` BucketClaim/BucketAccess provision the live bucket +
  `tuwunel-media-cosi-creds` BucketInfo JSON; ExternalSecret `tuwunel-s3`
  extracts `bucketName`/`accessKeyID`/`accessSecretKey` through the
  in-cluster `tuwunel-cosi` SecretStore.
- **CNPG backup S3 keys** (`cnpg-s3-credentials` Secret): COSI-minted.
  Same chain via `mautrix-discord-db` BucketClaim/Access +
  `mautrix-discord-db-cosi-creds` through `mautrix-discord-cosi`. (The
  old `pass://…/cnpg/s3-*` fallback entries may stay in the vault but are
  NOT referenced — rollback only.)
- **NetBird proxy outputs** (`element-proxy-outputs` Secret):
  `writeOutputsToSecret` of the `element-proxy` Terraform CR. Never in
  the vault, never in Git.
- **matrix-rooms bot fields** (`matrix-rooms/homeserver-url`,
  `bot-access-token`, `bot-user-id`): consumed by room-provisioning
  Terraform CRs that live in TEAM namespaces (see
  `matrix-rooms/examples/` + `matrix-rooms/terraform/README.md`), NOT by
  this tenant. Related but separate: the apprise fallback above uses
  `matrix-rooms/notifier-bot-token` + `homeserver-host` (same vault
  area, different fields).
- **In-namespace plumbing**: `eso-k8s-reader` RBAC, `kube-root-ca.crt`,
  the wildcard TLS Secret minted by cert-manager. No seeding.

## Cross-check (READMEs consulted)

- `base/tuwunel-secrets.yaml` + `base/tuwunel.yaml` header (SSO/S3
  contract) → rows: none vault-seeded; checks #1 (shared solver token).
- `base/mautrix-discord-secrets.yaml` header (7 tokens + db-password) →
  rows 4–11.
- `base/mautrix-discord-db.yaml` header (COSI chain + `cnpg/s3-*`
  fallback note) → not-seeded list.
- `base/apprise-go-api-secrets.yaml` header (token + host composition)
  → rows 2–3.
- `base/element-proxy.yaml` header (NetBird varsFrom) → rows 12–13.
- `base/NOTIFICATIONS.md` + pre-consolidation `tuwunel`/`mautrix-discord`
  READMEs (git history) → SSO Terraform follow-up + room-bot fields
  → not-seeded list.
