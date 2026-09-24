# matrix tenant — vault seed checklist

One-time `pass-cli item create` per env BEFORE first install. Git holds
`remoteRef` keys only, never values. The dev/prd overlays patch every
`remoteRef.key` below from the `__PROTON_PASS_BASE__` placeholder to the
per-env vault path; the Deployment/StatefulSet only starts once ESO syncs
the Secrets (pods pend + Flux retries until then).

Vault naming: `pass://acme-<env>-bdo1-talos-apps-01/<path>` where
`<env>` is `dev` or `prd`. The `pass-cli item create` shape drops the `pass://`
scheme (same convention as `matrix/terraform/README.md`):

```shell
pass-cli item create 'acme-<env>-bdo1-talos-apps-01/<path>'
```

Field count per env: **12** Proton Pass fields (1 cert-manager + 1
registration-secret + 8 mautrix-discord + 2 element-web). Matrix bot
credentials are minted in-cluster by the bootstrap Job, and coder reads
the kept Secret cross-namespace — neither needs vault seeding.
Everything else in this tenant is minted in-cluster (COSI, Terraform
outputs, CNPG, bootstrap kept Secret) and needs NO seeding — see
"Not vault-seeded" below.

## Per-env seed table

Same 12 rows for `dev` (`acme-dev-bdo1-talos-apps-01`) and `prd`
(`acme-prd-bdo1-talos-apps-01`); only the values differ per env (hosts,
tokens). Env-specific value notes are in the last column.

| # | Vault field (`<prefix>/<path>`) | ExternalSecret → Secret (key) | Consumed by | Value notes |
|---|---|---|---|---|
| 1 | `cert-manager/cloudflare-api-token` | `cloudflare-api-token` → `cloudflare-api-token` (`api-token`) | cert-manager DNS-01 solver for the in-namespace wildcard `Certificate` (`tuwunel-secrets.yaml`; mirrors `cert-manager/configs/base/cluster-issuer.yaml` with the same remoteRef) | Same Cloudflare API token in both envs (zone-scoped) |
| 2 | `matrix/tuwunel-registration-secret` | `tuwunel-registration-secret` → `tuwunel-registration-secret` (`shared-secret`) | Server side of the bot bootstrap: tuwunel Deployment mount (`TUWUNEL_REGISTRATION_SHARED_SECRET_FILE`) + `matrix-bot-bootstrap` Job env (`REGISTRATION_SHARED_SECRET`) — one source, no dual-write | Random ≥32 bytes; seeded ONCE per env, then stable forever (server + Job read the same Secret) |
| 3 | `mautrix-discord/bot-token` | `mautrix-discord` → `mautrix-discord` (`bot-token`) | Bridge entrypoint: `login-token bot <token>` session auth at runtime, never baked into config | Discord bot token from the Discord developer portal |
| 4 | `mautrix-discord/as-token` | (same ES/Secret, `as-token`) | Bridge registration (`registration.yaml` via `AS_TOKEN` env) + tuwunel appservice `as_token` side | Random ≥64ch; seeded ONCE, then stable forever (PVC persists `registration.yaml`) |
| 5 | `mautrix-discord/hs-token` | (same ES/Secret, `hs-token`) | Bridge registration (`HS_TOKEN` env) + tuwunel appservice `hs_token` side | Same stability contract as `as-token` |
| 6 | `mautrix-discord/avatar-proxy-key` | (same ES/Secret, `avatar-proxy-key`) | Bridge `/mautrix-discord/avatar` relay HMAC (`AVATAR_PROXY_KEY` env) | Random ≥32ch HMAC key |
| 7 | `mautrix-discord/direct-media-server-key` | (same ES/Secret, `direct-media-server-key`) | Bridge federation media signing (`DIRECT_MEDIA_SERVER_KEY` env; synapse `.signing.key` format) | Generate per synapse signing-key format |
| 8 | `mautrix-discord/provisioning-shared-secret` | (same ES/Secret, `provisioning-shared-secret`) | Bridge provisioning API auth (`PROVISIONING_SHARED_SECRET` env) | Random ≥32ch |
| 9 | `mautrix-discord/double-puppet-shared-secret` | (same ES/Secret, `double-puppet-shared-secret`) | Legacy `login_shared_secret_map` double-puppet value (`DOUBLE_PUPPET_SHARED_SECRET` env) | Random ≥32ch |
| 10 | `mautrix-discord/db-password` | `mautrix-discord-db-credentials` → `mautrix-discord-db-credentials` (`connection-url`, templated `postgres://discord:<pw>@mautrix-discord-db-rw.matrix.svc:5432/discord?sslmode=require`) AND `mautrix-discord-db-app-secret` → `mautrix-discord-db-app-secret` (basic-auth `discord`/`<pw>`; username MUST equal `spec.bootstrap.initdb.owner`) | Bridge `DATABASE_URL` env + CNPG `mautrix-discord-db` Cluster initdb owner password | Random ≥32ch; single field feeds BOTH Secrets |
| 11 | `element-web/netbird-pat` | `element-proxy-vars` → `element-proxy-vars` (`netbird_token`) | Terraform `element-proxy` CR via `varsFrom` (NetBird custom-domain + reverse-proxy service registration) | Per-env NetBird PAT |
| 12 | `element-web/cloudflare-api-token` | (same ES/Secret, `cloudflare_api_token`) | (same CR — Cloudflare side of the NetBird registration) | Distinct vault field from #1 (separate consumer), same upstream token value is fine |

Seed commands (dev shown; repeat with `acme-prd-bdo1-talos-apps-01` for prd):

```shell
pass-cli item create 'acme-dev-bdo1-talos-apps-01/cert-manager/cloudflare-api-token'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/matrix/tuwunel-registration-secret'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/mautrix-discord/bot-token'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/mautrix-discord/as-token'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/mautrix-discord/hs-token'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/mautrix-discord/avatar-proxy-key'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/mautrix-discord/direct-media-server-key'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/mautrix-discord/provisioning-shared-secret'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/mautrix-discord/double-puppet-shared-secret'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/mautrix-discord/db-password'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/element-web/netbird-pat'
pass-cli item create 'acme-dev-bdo1-talos-apps-01/element-web/cloudflare-api-token'
```

## Not vault-seeded (in-cluster minted — DO NOT `pass-cli item create`)

- **Tuwunel SSO `client_id`/`client_secret`**: NO `pass://` seeding.
  The companion `tuwunel-sso` Terraform CR (Zitadel project + `tuwunel`
  OIDC client, not in this tenant)
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
  `mautrix-discord-db-cosi-creds` through `mautrix-discord-cosi`.
- **NetBird proxy outputs** (`element-proxy-outputs` Secret):
  `writeOutputsToSecret` of the `element-proxy` Terraform CR. Never in
  the vault, never in Git.
- **matrix bot fields** (BOOTSTRAPPED — in-cluster minted, DO NOT
  `pass-cli item create`): the `matrix-bot-bootstrap` Job (`base/matrix-bot-bootstrap.yaml`)
  registers the per-env bot (`@apprise-dev` dev / `@apprise` prd, ONE bot
  shared by all 3 rooms) + mints its token + writes the KEPT Secret
  `matrix-bot-bootstrap-outputs` (keys `homeserver_url`/`access_token`/
  `user_id` for the rooms.yaml Terraform CRs via same-namespace `varsFrom`;
  keys `notifier-token`/`homeserver-host` for the apprise-stateless-urls ES
  via the in-cluster `tuwunel-k8s` store). The `matrix/terraform/examples/`
  files are copy-paste skeletons for TEAM namespaces only
  (`team-terraform.yaml` stays example-only — no team room here).
- **Coder notifier fields** (`coder/matrix-bot-token`, `coder/matrix-host`):
  not seeded — coder's `matrix-notify` ES reads the kept Secret
  (`notifier-token`/`homeserver-host`) cross-namespace (see the coder
  README credentials section).
- **In-namespace plumbing**: `eso-k8s-reader` RBAC, `kube-root-ca.crt`,
  the wildcard TLS Secret minted by cert-manager. No seeding.

Deleted vault paths (do not reseed): `matrix-rooms/*`, `coder/matrix-*`.
