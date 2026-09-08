# Zitadel (§14 dev)

Dev identity provider: the OIDC issuer for
`https://zitadel.homelab-dev.yansyah.my.id` plus the locked dev client
contract the §14 app writers build against (table below — do not deviate).
`kustomization.yaml` here references `../base` plus kustomize patches
carrying the §14 locked deltas: dev domain, DEV Proton Pass vault
(`pass://acme-dev-bdo1-talos-apps-01/...`), dev SeaweedFS S3 endpoints,
same DB/cache classes (3-instance CNPG, 3-replica Dragonfly,
`local-ssd-nvme`). No prd references outside comments.

## Layout

Patches against `configs/base`: ESO remoteRefs (dev vault),
namespace-local CNPG Cluster + ScheduledBackup (dev S3 endpoint),
namespace-local Dragonfly + snapshot credentials (dev S3 host),
in-namespace `wildcard-homelab-dev-tls`, HTTPRoutes (`/` → zitadel:8080,
`/ui/v2/login` → zitadel-login:3000 on the shared §14 DEV Gateway),
`org-users.yaml` intent (human-readable mirror) + `terraform-bootstrap.yaml`
(Terraform CR + ESO vars, machine-applied).
Controllers live in `controllers/dev` (`../base` + patch setting
`ExternalDomain: zitadel.homelab-dev.yansyah.my.id`).

## OIDC contract (LOCKED for §14 app writers)

App writers build against THIS table — do not deviate.

| Item | Value |
|---|---|
| Issuer | `https://zitadel.homelab-dev.yansyah.my.id` |
| Org | `home-ops` |
| Users | `admin@homelab-dev.yansyah.my.id` (super-admin, `admin` group + role), `user@homelab-dev.yansyah.my.id` (normal, `users` group + role) |
| Groups | `admin` (admin@ member), `users` (user@ member) — asserted in the `groups` claim |
| Scopes (all clients) | `openid profile email groups` |
| Flow (all clients) | Authorization code + PKCE, refresh tokens on |
| `clickstack` redirect | `https://clickstack.homelab-dev.yansyah.my.id/*` |
| `hubble` redirect | `https://hubble.homelab-dev.yansyah.my.id/*` |
| `flux-operator-ui` redirect | `https://flux-operator.homelab-dev.yansyah.my.id/*` |
| `headlamp` redirect | `https://headlamp.homelab-dev.yansyah.my.id/*` |
| `coder` redirect | `https://coder.homelab-dev.yansyah.my.id/*` |
| `oauth2-proxy-shared` redirect | `https://*/oauth2/callback` (ExternalAuth routes: clickstack, hubble, flux-operator-ui) |

Post-logout redirects point at each app's root (`https://<app>…/`).

## Identity-as-code (Tofu Controller) — DEV note

Machine-applied like base: the `zitadel-bootstrap-identity` Terraform CR
(applied from the same `configs/base/terraform-bootstrap.yaml`) carries the
DEV overrides via this overlay's `kustomization.yaml` patches — DEV vault
refs (`pass://acme-dev-bdo1-talos-apps-01/zitadel/terraform-*`) plus CR
`vars`:

- `domain=zitadel.homelab-dev.yansyah.my.id`
- `admin_email=admin@homelab-dev.yansyah.my.id`
- `user_email=user@homelab-dev.yansyah.my.id`
- Client `redirect_uris`/`post_logout_redirect_uris` locals: replace the
  prd domain with `homelab-dev.yansyah.my.id` (same shapes as the table
  above); `oauth2-proxy-shared` stays `https://*/oauth2/callback`.
- Read the generated client secrets from state into the DEV Proton Pass
  vault (`pass://acme-dev-bdo1-talos-apps-01/...`, never Git) — each §14
  app's dev ESO secrets reference those entries.

## Credentials

All secrets sync from Proton Pass via ESO (Git holds `remoteRef`s only).
Seed each vault entry with pass-cli (never commit):

- `pass://acme-dev-bdo1-talos-apps-01/zitadel/masterkey` — 32-byte masterkey.
  IMMUTABLE: Zitadel cannot re-key; loss means loss of all encrypted data.
- `pass://acme-dev-bdo1-talos-apps-01/zitadel/db-dsn` — full DSN, must embed
  the same password as `db-password`.
- `pass://acme-dev-bdo1-talos-apps-01/zitadel/db-password` — CNPG app-user
  password (username must equal `initdb.owner`; rotate with the DSN).
- `pass://acme-dev-bdo1-talos-apps-01/zitadel/smtp-*` — relay user/password
  (optional; unwired until a relay exists).
- `pass://acme-dev-bdo1-talos-apps-01/{cnpg,dragonfly}/s3-*` and
  `pass://acme-dev-bdo1-talos-apps-01/cert-manager/cloudflare-api-token` —
  same vault paths as the §14 DEV §§9–10, copied so the Barman/snapshot/DNS-01
  secrets exist in this namespace too.
